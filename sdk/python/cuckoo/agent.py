"""The Agent: a socket to the hub, a handler, and ways to reply."""

from __future__ import annotations

import asyncio
import json
import logging
import mimetypes
import os
import uuid
from collections import OrderedDict
from collections.abc import Awaitable, Callable
from pathlib import Path
from typing import Any, Self

import httpx
import websockets
from websockets.exceptions import ConnectionClosed, InvalidStatus

from .errors import ProtocolError, StoppedError, error_of, raise_for_status
from .models import DEFAULT_HUB, Attachment, Conversation, Message, PairToken, StopRequest
from .schedules import Schedule, ScheduleChange
from .stream import Stream
from .wire import Attachable, Buttons, _buttons_json, _quick_replies_json

log = logging.getLogger("cuckoo")

# Close codes the hub uses to say "do not come back".
CLOSE_REPLACED = 4001
CLOSE_BINDING_GONE = 4002

# How many event ids to remember for de-duplication. The hub redelivers an
# event it never saw acknowledged, so a slow ack can mean seeing one twice.
_REMEMBER = 1000

# How long to wait for the hub to answer a socket operation.
_REPLY_TIMEOUT = 30.0

Handler = Callable[[Message, Conversation], Awaitable[None]]
JoinHandler = Callable[[Conversation, PairToken | None], Awaitable[None]]
StopHandler = Callable[[StopRequest, Conversation], Awaitable[None]]
ScheduleHandler = Callable[[ScheduleChange, Conversation], Awaitable[None]]

__all__ = ["Agent", "Attachable", "Buttons", "ProtocolError", "Stream", "StoppedError"]

class Agent:
    """A backend for one agent identity.

    ``secret`` is the binding secret shown once when the binding was set.
    ``hub`` is the hub's base URL; ``http://localhost:8080`` for a local one.
    """

    def __init__(
        self, secret: str, hub: str = DEFAULT_HUB, *, max_backoff: float = 30.0
    ):
        if not secret.startswith("bnd_sec_"):
            raise ValueError("secret must be a binding secret, which starts with bnd_sec_")
        self._secret = secret
        self._hub = hub.rstrip("/")
        self._max_backoff = max_backoff
        self._handler: Handler | None = None
        self._join_handler: JoinHandler | None = None
        self._stop_handler: StopHandler | None = None
        self._schedule_handler: ScheduleHandler | None = None
        self._seen: OrderedDict[str, None] = OrderedDict()
        # Message handlers running now, by conversation, so a stop reaches
        # them; and the replies the hub says were stopped, so a stream's
        # next append fails here rather than disappearing on the socket.
        self._running: dict[str, dict[asyncio.Task[None], Conversation]] = {}
        self._stopped: OrderedDict[str, None] = OrderedDict()
        self._http: httpx.AsyncClient | None = None
        self._ws: Any = None
        self._pending: dict[str, asyncio.Future[dict[str, Any]]] = {}
        self._tasks: set[asyncio.Task[None]] = set()

    # -- registration -----------------------------------------------------

    def on_message(self, fn: Handler) -> Handler:
        """Register the coroutine called for every message the agent receives."""
        self._handler = fn
        return fn

    def on_join(self, fn: JoinHandler) -> JoinHandler:
        """Register the coroutine called when someone adds the agent: a new
        conversation exists, and the token they scanned, if any, says who they
        are on your side. The place to say hello first."""
        self._join_handler = fn
        return fn

    def on_stop(self, fn: StopHandler) -> StopHandler:
        """Register the coroutine called when the person presses stop.

        Without one, stopping still works: the handler running for that
        conversation is cancelled, and a write from it raises StoppedError,
        which ends it quietly. What is left for this handler is to say
        something true — "Stopped. Nothing was booked." — if anything.
        """
        self._stop_handler = fn
        return fn

    def on_schedule(self, fn: ScheduleHandler) -> ScheduleHandler:
        """Register the coroutine called when the person makes, changes or
        deletes a schedule in the app. Hold it in your own timer, then
        ``await conv.schedules.confirm(change.schedule.id)``; the app says
        "waiting" until you do."""
        self._schedule_handler = fn
        return fn

    # -- running ----------------------------------------------------------

    def run(self) -> None:
        """Connect and serve until interrupted. Blocks."""
        try:
            asyncio.run(self.serve())
        except KeyboardInterrupt:
            log.info("stopped")

    async def serve(self) -> None:
        """Connect and serve, reconnecting when the connection drops."""
        if self._handler is None:
            raise RuntimeError("register a handler with @agent.on_message before running")

        backoff = 1.0
        async with httpx.AsyncClient(
            base_url=self._hub, headers=self._auth(), timeout=30.0
        ) as http:
            self._http = http
            while True:
                started = asyncio.get_running_loop().time()
                try:
                    await self._session()
                    return
                except InvalidStatus as exc:
                    status = exc.response.status_code
                    if status in (401, 403):
                        log.error(
                            "hub refused the connection (%s); check the secret and binding mode",
                            status,
                        )
                        return
                    log.warning("handshake failed with %s; retrying", status)
                except (OSError, ConnectionClosed) as exc:
                    log.warning("connection lost (%s); reconnecting", exc)

                # A connection that lasted a while earned a fresh backoff.
                if asyncio.get_running_loop().time() - started > 60:
                    backoff = 1.0
                await asyncio.sleep(backoff)
                backoff = min(backoff * 2, self._max_backoff)

    async def _session(self) -> None:
        url = (
            self._hub.replace("https://", "wss://", 1).replace("http://", "ws://", 1)
            + "/v1/agent/socket"
        )
        try:
            async with websockets.connect(url, additional_headers=self._auth()) as ws:
                self._ws = ws
                log.info("connected to %s", self._hub)
                async for raw in ws:
                    self._dispatch(ws, json.loads(raw))
        except ConnectionClosed as exc:
            if exc.rcvd is not None and exc.rcvd.code == CLOSE_REPLACED:
                log.error("another connection took over this binding; stopping")
                return
            if exc.rcvd is not None and exc.rcvd.code == CLOSE_BINDING_GONE:
                log.error("the binding was revoked or replaced; stopping")
                return
            raise
        finally:
            self._ws = None
            for fut in self._pending.values():
                if not fut.done():
                    fut.set_exception(ConnectionError("connection closed before the hub answered"))
            self._pending.clear()

    def _dispatch(self, ws: Any, frame: dict[str, Any]) -> None:
        """Route one frame: an answer to us, a rejection, or an event."""
        cid = frame.get("reply_to_cid")
        if cid is not None:
            fut = self._pending.pop(cid, None)
            if fut is not None and not fut.done():
                fut.set_result(frame)
            return
        if "error" in frame and "type" not in frame:
            error = frame["error"] or {}
            if error.get("code") == "stopped" and frame.get("message_id"):
                self._mark_stopped(frame["message_id"])
                return
            log.warning("hub rejected a frame for %s: %s", frame.get("message_id"), error)
            return
        if frame.get("id") and frame.get("type"):
            # Handlers run as tasks so a slow one never blocks the frames the
            # others, and our own operations, are waiting for.
            task = asyncio.create_task(self._handle(ws, frame))
            self._tasks.add(task)
            task.add_done_callback(self._tasks.discard)

    async def _handle(self, ws: Any, event: dict[str, Any]) -> None:
        event_id = event["id"]
        if event_id in self._seen:
            await self._ack(ws, event_id)
            return
        if event.get("type") == "conversation.joined" and self._join_handler is not None:
            data = event.get("data") or {}
            conv = Conversation.from_wire(
                data["conversation"], data.get("participants") or [], self
            )
            try:
                await self._join_handler(conv, PairToken.from_wire(data.get("pair_token")))
            except Exception:  # a handler bug must not stop the agent
                log.exception("join handler failed for %s; the hub will send it again", event_id)
                return
            self._remember(event_id)
            await self._ack(ws, event_id)
            return
        if str(event.get("type", "")).startswith("schedule.") and self._schedule_handler is not None:
            data = event.get("data") or {}
            conv = Conversation.from_wire(data["conversation"], data.get("participants") or [], self)
            change = ScheduleChange(
                event["type"].removeprefix("schedule."), Schedule.from_wire(data.get("schedule") or {})
            )
            try:
                await self._schedule_handler(change, conv)
            except Exception:  # a handler bug must not stop the agent
                log.exception("schedule handler failed for %s; the hub will send it again", event_id)
                return
            self._remember(event_id)
            await self._ack(ws, event_id)
            return
        if event.get("type") == "stop.requested":
            if not await self._stop(event):
                return
            self._remember(event_id)
            await self._ack(ws, event_id)
            return
        if event.get("type") != "message.created":
            # Nothing to do with it, but it was received.
            self._remember(event_id)
            await self._ack(ws, event_id)
            return

        data = event.get("data") or {}
        conv = Conversation.from_wire(data["conversation"], data.get("participants") or [], self)
        msg = Message.from_wire(data["message"], conv.id, event_id, self)
        assert self._handler is not None
        task = asyncio.current_task()
        assert task is not None
        self._running.setdefault(conv.id, {})[task] = conv
        try:
            await self._handler(msg, conv)
        except asyncio.CancelledError:
            if not conv.stopped:
                raise
            # The person pressed stop: the work is over, as they asked, and
            # this cancellation was ours rather than the agent shutting down.
            task.uncancel()
        except StoppedError:
            pass
        except Exception:  # a handler bug must not stop the agent
            log.exception("handler failed for %s; the hub will send it again", event_id)
            return
        finally:
            running = self._running.get(conv.id, {})
            running.pop(task, None)
            if not running:
                self._running.pop(conv.id, None)

        self._remember(event_id)
        await self._ack(ws, event_id)

    async def _ack(self, ws: Any, event_id: str) -> None:
        try:
            await ws.send(json.dumps({"ack": event_id}))
        except ConnectionClosed:
            log.warning("could not acknowledge %s; the hub will send it again", event_id)

    def _remember(self, event_id: str) -> None:
        self._seen[event_id] = None
        while len(self._seen) > _REMEMBER:
            self._seen.popitem(last=False)

    async def _stop(self, event: dict[str, Any]) -> bool:
        """A person pressed stop: cancel what runs for the conversation, then
        ask the stop handler. Reports whether the event is done with."""
        data = event.get("data") or {}
        conversation = data.get("conversation") or {}
        conversation_id = conversation.get("id", "")
        message_id = data.get("message_id")
        if message_id:
            self._mark_stopped(message_id)
        for task, conv in list(self._running.get(conversation_id, {}).items()):
            conv.stopped = True
            task.cancel()
        if self._stop_handler is None:
            return True
        conv = Conversation.from_wire(conversation, [], self)
        try:
            await self._stop_handler(StopRequest(conversation_id, message_id), conv)
        except Exception:  # a handler bug must not stop the agent
            log.exception("stop handler failed for %s; the hub will send it again", event["id"])
            return False
        return True

    def _mark_stopped(self, message_id: str) -> None:
        self._stopped[message_id] = None
        while len(self._stopped) > _REMEMBER:
            self._stopped.popitem(last=False)

    def _was_stopped(self, message_id: str) -> bool:
        return message_id in self._stopped

    # -- sending ----------------------------------------------------------

    async def send(
        self,
        conversation_id: str,
        text: str = "",
        *,
        attachments: list[Attachable] | None = None,
        buttons: Buttons | None = None,
        quick_replies: list[str] | None = None,
        reply_to: str | None = None,
        idempotency_key: str | None = None,
        schedule_id: str | None = None,
    ) -> Message:
        """Say something in a conversation the agent is in.

        Anything in ``attachments`` that is a path is uploaded first, and the
        message names what was uploaded. A message carrying files needs no
        text; the text, if there is any, is their caption.
        """
        if self._http is None:
            raise RuntimeError("send is only available while the agent is running")
        body: dict[str, Any] = {
            "text": text,
            "idempotency_key": idempotency_key or str(uuid.uuid4()),
        }
        if media_ids := await self._upload_all(attachments):
            body["attachments"] = media_ids
        if reply_to:
            body["reply_to"] = reply_to
        if schedule_id:
            body["schedule_id"] = schedule_id
        if wire := _buttons_json(buttons):
            body["buttons"] = wire
        if wire := _quick_replies_json(quick_replies):
            body["quick_replies"] = wire
        response = await self._http.post(
            f"/v1/agent/conversations/{conversation_id}/messages", json=body
        )
        raise_for_status(response)
        return Message.from_wire(response.json()["message"], conversation_id, agent=self)

    # -- files ------------------------------------------------------------

    async def upload(
        self,
        path: str | os.PathLike[str],
        *,
        duration_ms: int = 0,
        waveform: list[int] | None = None,
        audio: bool = False,
    ) -> Attachment:
        """Put a file on the hub, ready to be sent.

        Sending is a separate step, so a slow upload does not hold a message
        open. An upload nobody sends is removed after a day.

        For a recording, ``duration_ms`` and ``waveform`` (0 to 100, a few
        dozen values) are what the app draws the bubble from without
        decoding any audio. ``audio=True`` says a WebM, Ogg or MP4 file is a
        recording rather than a video, which nothing can tell from the bytes.
        """
        if self._http is None:
            raise RuntimeError("upload is only available while the agent is running")
        file = Path(path)
        content_type = mimetypes.guess_type(file.name)[0] or "application/octet-stream"
        params: dict[str, str] = {}
        if duration_ms:
            params["duration_ms"] = str(int(duration_ms))
        if waveform:
            params["waveform"] = ",".join(str(int(n)) for n in waveform)
        if audio or content_type.startswith("audio/"):
            params["kind"] = "audio"
        # Read off the event loop: a file on a disk blocks, and a backend
        # answering other conversations should not stop while this one reads.
        data = await asyncio.to_thread(file.read_bytes)
        response = await self._http.post(
            "/v1/agent/media",
            params=params or None,
            files={"file": (file.name, data, content_type)},
            timeout=None,
        )
        raise_for_status(response)
        return Attachment.from_wire(response.json()["media"], self)

    async def download(self, media_id: str, *, thumbnail: bool = False) -> bytes:
        """Fetch the bytes of a file on a message in one of this agent's
        conversations."""
        if self._http is None:
            raise RuntimeError("download is only available while the agent is running")
        response = await self._http.get(
            f"/v1/agent/media/{media_id}",
            params={"variant": "thumb"} if thumbnail else None,
            timeout=None,
        )
        raise_for_status(response)
        return response.content

    async def _upload_all(self, attachments: list[Attachable] | None) -> list[str]:
        """Turn what a caller passed into media ids, uploading the paths."""
        if not attachments:
            return []
        ids: list[str] = []
        for item in attachments:
            if isinstance(item, Attachment):
                ids.append(item.media_id)
            elif isinstance(item, str) and item.startswith("med_"):
                ids.append(item)
            else:
                ids.append((await self.upload(item)).media_id)
        return ids

    def stream(
        self,
        conversation_id: str,
        *,
        reply_to: str | None = None,
        buttons: Buttons | None = None,
        quick_replies: list[str] | None = None,
        schedule_id: str | None = None,
    ) -> Stream:
        """Begin a message that arrives piece by piece. Use as ``async with``."""
        return Stream(
            self,
            conversation_id,
            reply_to=reply_to,
            buttons=buttons,
            quick_replies=quick_replies,
            schedule_id=schedule_id,
        )

    async def typing(self, conversation_id: str, state: str = "start") -> None:
        """Show, or hide, the "working" indicator on the person's device."""
        await self._call("typing", conversation_id=conversation_id, state=state)

    async def activity(self, conversation_id: str, state: str, label: str | None = None) -> None:
        """Say what the agent is doing: ``thinking``, ``working`` with a short
        label the person sees ("Checking the weather"), or ``idle``. It shows
        for ten seconds unless said again; ``Conversation.working`` keeps it up."""
        fields: dict[str, Any] = {"conversation_id": conversation_id, "state": state}
        if label:
            fields["label"] = label
        await self._call("activity", **fields)

    # -- socket operations ------------------------------------------------

    async def _call(self, op: str, **fields: Any) -> dict[str, Any]:
        """Send an operation over the socket and wait for the hub's answer."""
        ws = self._ws
        if ws is None:
            raise ConnectionError("not connected to the hub")
        cid = uuid.uuid4().hex
        fut: asyncio.Future[dict[str, Any]] = asyncio.get_running_loop().create_future()
        self._pending[cid] = fut
        try:
            await ws.send(json.dumps({"op": op, "cid": cid, **fields}))
            reply = await asyncio.wait_for(fut, _REPLY_TIMEOUT)
        finally:
            self._pending.pop(cid, None)
        if not reply.get("ok"):
            err = reply.get("error") or {}
            raise error_of(
                err.get("code", "unknown"), err.get("message", "The hub refused the operation.")
            )
        return reply

    async def _fire(self, op: str, **fields: Any) -> None:
        """Send an operation the hub only answers when it fails."""
        ws = self._ws
        if ws is None:
            raise ConnectionError("not connected to the hub")
        await ws.send(json.dumps({"op": op, **fields}))

    def _auth(self) -> dict[str, str]:
        return {"Authorization": f"Bearer {self._secret}"}
