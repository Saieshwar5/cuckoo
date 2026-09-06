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

from .models import Attachment, Conversation, Link, Message, PairToken

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

# Rows of buttons. A button is either a choice, whose id comes back when it
# is tapped — (id, label), (id, label, style), or a dict with those keys —
# or a Link, which opens something and tells you nothing.
Buttons = list[list[tuple[str, str] | tuple[str, str, str] | dict[str, str] | Link]]

# What can be sent as an attachment: a path to a file, or something already
# uploaded — an Attachment from a message, or a media id.
Attachable = str | os.PathLike[str] | Attachment


def _buttons_json(buttons: Buttons | None) -> list[list[dict[str, str]]] | None:
    if not buttons:
        return None
    rows = []
    for row in buttons:
        wire = []
        for b in row:
            if isinstance(b, Link):
                wire.append({"url": b.url, "label": b.label, "style": b.style})
            elif isinstance(b, dict):
                entry = {"label": b["label"], "style": b.get("style", "default")}
                # A url button has no id, and an id button no url: the hub
                # refuses a button carrying both.
                entry["url" if "url" in b else "id"] = b.get("url") or b["id"]
                wire.append(entry)
            else:
                wire.append({"id": b[0], "label": b[1], "style": b[2] if len(b) > 2 else "default"})
        rows.append(wire)
    return rows


def _quick_replies_json(labels: list[str] | None) -> list[dict[str, str]] | None:
    if not labels:
        return None
    return [{"label": label} for label in labels]


class ProtocolError(Exception):
    """The hub refused an operation. ``code`` is the stable error code."""

    def __init__(self, code: str, message: str):
        super().__init__(f"{code}: {message}")
        self.code = code
        self.message = message


def _raise_for_status(response: httpx.Response) -> None:
    """Turn a refusal into the hub's own error, so a caller sees
    "file_too_large" rather than "422 Unprocessable Entity"."""
    if response.status_code < 400:
        return
    try:
        error = response.json()["error"]
    except Exception:  # noqa: BLE001 - a hub that answered with prose
        raise ProtocolError(
            "http_error", f"{response.status_code}: {response.text[:200]}"
        ) from None
    raise ProtocolError(error.get("code", "error"), error.get("message", ""))


class Agent:
    """A backend for one agent identity.

    ``secret`` is the binding secret shown once when the binding was set.
    ``hub`` is the hub's base URL; ``http://localhost:8080`` for a local one.
    """

    def __init__(
        self, secret: str, hub: str = "https://api.cuckoo.in", *, max_backoff: float = 30.0
    ):
        if not secret.startswith("bnd_sec_"):
            raise ValueError("secret must be a binding secret, which starts with bnd_sec_")
        self._secret = secret
        self._hub = hub.rstrip("/")
        self._max_backoff = max_backoff
        self._handler: Handler | None = None
        self._join_handler: JoinHandler | None = None
        self._seen: OrderedDict[str, None] = OrderedDict()
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
            log.warning("hub rejected a frame for %s: %s", frame.get("message_id"), frame["error"])
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
        if event.get("type") != "message.created":
            # Nothing to do with it, but it was received.
            self._remember(event_id)
            await self._ack(ws, event_id)
            return

        data = event.get("data") or {}
        conv = Conversation.from_wire(data["conversation"], data.get("participants") or [], self)
        msg = Message.from_wire(data["message"], conv.id, event_id, self)
        assert self._handler is not None
        try:
            await self._handler(msg, conv)
        except Exception:  # a handler bug must not stop the agent
            log.exception("handler failed for %s; the hub will send it again", event_id)
            return

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
        if wire := _buttons_json(buttons):
            body["buttons"] = wire
        if wire := _quick_replies_json(quick_replies):
            body["quick_replies"] = wire
        response = await self._http.post(
            f"/v1/agent/conversations/{conversation_id}/messages", json=body
        )
        _raise_for_status(response)
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
        _raise_for_status(response)
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
        _raise_for_status(response)
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
    ) -> Stream:
        """Begin a message that arrives piece by piece. Use as ``async with``."""
        return Stream(
            self, conversation_id, reply_to=reply_to, buttons=buttons, quick_replies=quick_replies
        )

    async def typing(self, conversation_id: str, state: str = "start") -> None:
        """Show, or hide, the "working" indicator on the person's device."""
        await self._call("typing", conversation_id=conversation_id, state=state)

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
            raise ProtocolError(
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


class Stream:
    """A message being written. Entering starts it; ``append`` adds text;
    leaving finishes it, even after an exception, with what was sent."""

    def __init__(
        self,
        agent: Agent,
        conversation_id: str,
        *,
        reply_to: str | None = None,
        buttons: Buttons | None = None,
        quick_replies: list[str] | None = None,
    ):
        self._agent = agent
        self.conversation_id = conversation_id
        self.reply_to = reply_to
        self.buttons = buttons
        self.quick_replies = quick_replies
        self.message_id: str | None = None
        self.message: Message | None = None

    async def __aenter__(self) -> Self:
        fields: dict[str, Any] = {"conversation_id": self.conversation_id}
        if self.reply_to:
            fields["reply_to"] = self.reply_to
        reply = await self._agent._call("stream.start", **fields)
        self.message_id = reply["message"]["id"]
        return self

    async def append(self, text: str) -> None:
        """Add text to the message."""
        if self.message_id is None:
            raise RuntimeError("append is only available inside the stream's context")
        await self._agent._fire("stream.delta", message_id=self.message_id, text=text)

    async def __aexit__(self, exc_type: object, exc: object, tb: object) -> bool:
        if self.message_id is not None:
            fields: dict[str, Any] = {"message_id": self.message_id}
            if wire := _buttons_json(self.buttons):
                fields["buttons"] = wire
            if wire := _quick_replies_json(self.quick_replies):
                fields["quick_replies"] = wire
            reply = await self._agent._call("stream.end", **fields)
            self.message = Message.from_wire(reply["message"], self.conversation_id)
        return False
