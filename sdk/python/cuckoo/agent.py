"""The Agent: a socket to the hub, a handler, and a way to reply."""

from __future__ import annotations

import asyncio
import json
import logging
import uuid
from collections import OrderedDict
from collections.abc import Awaitable, Callable
from typing import Any

import httpx
import websockets
from websockets.exceptions import ConnectionClosed, InvalidStatus

from .models import Conversation, Message

log = logging.getLogger("cuckoo")

# Close codes the hub uses to say "do not come back".
CLOSE_REPLACED = 4001
CLOSE_BINDING_GONE = 4002

# How many event ids to remember for de-duplication. The hub redelivers an
# event it never saw acknowledged, so a slow ack can mean seeing one twice.
_REMEMBER = 1000

Handler = Callable[[Message, Conversation], Awaitable[None]]


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
        self._seen: OrderedDict[str, None] = OrderedDict()
        self._http: httpx.AsyncClient | None = None

    # -- registration -----------------------------------------------------

    def on_message(self, fn: Handler) -> Handler:
        """Register the coroutine called for every message the agent receives."""
        self._handler = fn
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
                log.info("connected to %s", self._hub)
                async for raw in ws:
                    await self._handle(ws, json.loads(raw))
        except ConnectionClosed as exc:
            if exc.rcvd is not None and exc.rcvd.code == CLOSE_REPLACED:
                log.error("another connection took over this binding; stopping")
                return
            if exc.rcvd is not None and exc.rcvd.code == CLOSE_BINDING_GONE:
                log.error("the binding was revoked or replaced; stopping")
                return
            raise

    async def _handle(self, ws: Any, event: dict[str, Any]) -> None:
        event_id = event.get("id")
        if not event_id:
            return
        if event_id in self._seen:
            await ws.send(json.dumps({"ack": event_id}))
            return

        if event.get("type") != "message.created":
            # Nothing to do with it, but it was received.
            self._remember(event_id)
            await ws.send(json.dumps({"ack": event_id}))
            return

        data = event.get("data") or {}
        conv = Conversation.from_wire(data["conversation"], data.get("participants") or [], self)
        msg = Message.from_wire(data["message"], conv.id, event_id)
        assert self._handler is not None
        try:
            await self._handler(msg, conv)
        except Exception:  # a handler bug must not stop the agent
            log.exception("handler failed for %s; the hub will send it again", event_id)
            return

        self._remember(event_id)
        await ws.send(json.dumps({"ack": event_id}))

    def _remember(self, event_id: str) -> None:
        self._seen[event_id] = None
        while len(self._seen) > _REMEMBER:
            self._seen.popitem(last=False)

    # -- sending ----------------------------------------------------------

    async def send(
        self, conversation_id: str, text: str, *, idempotency_key: str | None = None
    ) -> Message:
        """Say something in a conversation the agent is in."""
        if self._http is None:
            raise RuntimeError("send is only available while the agent is running")
        response = await self._http.post(
            f"/v1/agent/conversations/{conversation_id}/messages",
            json={"text": text, "idempotency_key": idempotency_key or str(uuid.uuid4())},
        )
        response.raise_for_status()
        return Message.from_wire(response.json()["message"], conversation_id)

    def _auth(self) -> dict[str, str]:
        return {"Authorization": f"Bearer {self._secret}"}
