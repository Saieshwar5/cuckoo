"""What a handler is given: the message, and the conversation to reply into."""

from __future__ import annotations

import asyncio
import os
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager, suppress
from dataclasses import dataclass, field
from pathlib import Path
from typing import TYPE_CHECKING, Any

from .errors import StoppedError

if TYPE_CHECKING:
    from .agent import Agent
    from .stream import Stream
    from .wire import Attachable, Buttons

# An activity lasts ten seconds on the hub; it is said again well before.
_RENEW_SECONDS = 7.0


# The public hub, and the default everywhere in this package. A development
# hub is http://localhost:8080; pass it explicitly.
DEFAULT_HUB = "https://cuckoo.onl"


@dataclass(frozen=True)
class Sender:
    """Who wrote a message. A backend never learns more about a person."""

    kind: str
    id: str
    display_name: str


@dataclass(frozen=True)
class Participant:
    """A current member of a conversation. ``is_me`` marks your own agent."""

    kind: str
    id: str
    display_name: str
    handle: str | None = None
    is_me: bool = False


@dataclass(frozen=True)
class Link:
    """A button that opens something instead of answering.

    A tracking page, a UPI payment, a phone number, your own site. The tap
    tells you nothing and never arrives as a message — the page you sent
    them to is yours, and that is where you learn what happened. It is also
    how a poster's QR code, which cannot say who scanned it, gets a person
    to sign in on your side and become someone you know.

    ``url`` starts with ``https``, ``http``, ``upi`` or ``tel``.
    """

    label: str
    url: str
    style: str = "default"


@dataclass(frozen=True)
class Action:
    """A tap: which button, on which of your messages."""

    button_id: str
    source_message_id: str


@dataclass(frozen=True)
class ReplyRef:
    """The message a message answers, with a preview of it."""

    id: str
    sender_kind: str
    text_preview: str


@dataclass(frozen=True)
class Attachment:
    """A file on a message: a photo of a bill, a voice note, a document.

    The bytes are not here. ``download`` fetches them when you want them,
    which matters when the message is a 40 MB video and your handler only
    wanted to read the caption.
    """

    media_id: str
    kind: str
    mime_type: str
    file_name: str
    byte_size: int = 0
    width: int = 0
    height: int = 0
    has_thumbnail: bool = False
    # For a recording or a video: how long it runs, and its loudness over
    # time from 0 to 100. Both come from the device that recorded it — the
    # hub does not decode audio to check, and neither should you.
    duration_ms: int = 0
    waveform: tuple[int, ...] = ()
    _agent: Agent | None = field(default=None, repr=False, compare=False)

    @property
    def is_image(self) -> bool:
        return self.kind == "image"

    @property
    def is_audio(self) -> bool:
        return self.kind == "audio"

    @property
    def seconds(self) -> float:
        """How long a recording or video runs. Zero for anything else."""
        return self.duration_ms / 1000

    @classmethod
    def from_wire(cls, data: dict[str, Any], agent: Agent | None = None) -> Attachment:
        return cls(
            # An upload calls it id; a message calls it media_id. Same file.
            media_id=data.get("media_id") or data.get("id", ""),
            kind=data.get("kind", ""),
            mime_type=data.get("mime_type", ""),
            file_name=data.get("file_name", ""),
            byte_size=int(data.get("byte_size") or 0),
            width=int(data.get("width") or 0),
            height=int(data.get("height") or 0),
            has_thumbnail=bool(data.get("has_thumbnail")),
            duration_ms=int(data.get("duration_ms") or 0),
            waveform=tuple(int(n) for n in (data.get("waveform") or ())),
            _agent=agent,
        )

    async def download(self, *, thumbnail: bool = False) -> bytes:
        """Fetch the bytes. ``thumbnail=True`` gets the small copy of a
        picture, which is enough to look at and a fraction of the size."""
        return await self._attached().download(self.media_id, thumbnail=thumbnail)

    async def save(self, path: str | os.PathLike[str], *, thumbnail: bool = False) -> Path:
        """Write the bytes to a file, or into a directory under the name the
        sender gave it. Returns where it went."""
        target = Path(path)
        if target.is_dir():
            target = target / (self.file_name or self.media_id)
        target.write_bytes(await self.download(thumbnail=thumbnail))
        return target

    def _attached(self) -> Agent:
        if self._agent is None:
            raise RuntimeError("this attachment is not attached to a running agent")
        return self._agent


@dataclass(frozen=True)
class Message:
    """One thing said in a conversation. ``action`` is set when the person
    tapped one of your buttons; ``text`` is then the button's label.

    ``signature`` is the hub's own seal over the message, made with a key
    only the hub holds. Keep it with the message and leave it unchanged; it
    is what lets a copy you kept past the hub's window be served back later.
    """

    id: str
    conversation_id: str
    text: str
    sender: Sender
    created_at: str
    status: str = "complete"
    truncated: bool = False
    # The person pressed stop while this was being written.
    stopped: bool = False
    action: Action | None = None
    reply_to: ReplyRef | None = None
    # Opaque; absent on messages from before the hub signed them.
    signature: str | None = None
    event_id: str | None = None
    attachments: tuple[Attachment, ...] = ()

    @property
    def has_attachments(self) -> bool:
        return bool(self.attachments)

    @classmethod
    def from_wire(
        cls,
        data: dict[str, Any],
        conversation_id: str,
        event_id: str | None = None,
        agent: Agent | None = None,
    ) -> Message:
        sender = data.get("sender") or {}
        body = data.get("body") or {}
        action = body.get("action")
        reply = data.get("reply_to")
        return cls(
            id=data["id"],
            conversation_id=conversation_id,
            text=(data.get("body") or {}).get("text", ""),
            sender=Sender(
                kind=sender.get("kind", ""),
                id=sender.get("id", ""),
                display_name=sender.get("display_name", ""),
            ),
            created_at=data.get("created_at", ""),
            status=data.get("status", "complete"),
            truncated=bool(data.get("truncated")),
            stopped=bool(data.get("stopped")),
            action=Action(action["button_id"], action["source_message_id"]) if action else None,
            reply_to=ReplyRef(
                reply["id"], reply.get("sender_kind", ""), reply.get("text_preview", "")
            )
            if reply
            else None,
            signature=data.get("signature") or None,
            event_id=event_id,
            attachments=tuple(
                Attachment.from_wire(a, agent) for a in (body.get("attachments") or [])
            ),
        )


@dataclass(frozen=True)
class StopRequest:
    """A person pressing stop. ``message_id`` is the reply the hub ended, if
    the agent had started writing one."""

    conversation_id: str
    message_id: str | None = None


@dataclass
class PairToken:
    """The code someone scanned to reach the agent, with whatever the owner
    put in it: a personalised code carries the company's own reference, a
    poster carries nothing."""

    id: str
    payload: Any = None

    @classmethod
    def from_wire(cls, data: dict[str, Any] | None) -> PairToken | None:
        if not data:
            return None
        return cls(id=data.get("id", ""), payload=data.get("payload"))


@dataclass
class Conversation:
    """Where a message was said, and the handle for replying there."""

    id: str
    kind: str
    participants: list[Participant] = field(default_factory=list)
    _agent: Agent | None = field(default=None, repr=False, compare=False)
    # Set when the person presses stop while a handler works on this
    # conversation. Every write from here on raises StoppedError.
    stopped: bool = field(default=False, compare=False)

    @classmethod
    def from_wire(
        cls, data: dict[str, Any], participants: list[dict[str, Any]], agent: Agent
    ) -> Conversation:
        return cls(
            id=data["id"],
            kind=data.get("kind", ""),
            participants=[
                Participant(
                    kind=p.get("kind", ""),
                    id=p.get("id", ""),
                    display_name=p.get("display_name", ""),
                    handle=p.get("handle"),
                    is_me=bool(p.get("is_me")),
                )
                for p in participants
            ],
            _agent=agent,
        )

    async def send(
        self,
        text: str = "",
        *,
        attachments: list[Attachable] | None = None,
        buttons: Buttons | None = None,
        quick_replies: list[str] | None = None,
        reply_to: str | None = None,
        idempotency_key: str | None = None,
    ) -> Message:
        """Say something in this conversation.

        ``attachments`` are paths to files, or things already uploaded; they
        are uploaded and sent with the message, and ``text`` becomes their
        caption and may be empty. ``buttons`` is rows of ``(id, label)`` or
        ``(id, label, style)``; the person's tap comes back as a message
        whose ``action.button_id`` is the id. ``quick_replies`` are suggested
        answers sent as plain text.
        """
        return await self._writable().send(
            self.id,
            text,
            attachments=attachments,
            buttons=buttons,
            quick_replies=quick_replies,
            reply_to=reply_to,
            idempotency_key=idempotency_key,
        )

    def stream(
        self,
        *,
        reply_to: str | None = None,
        buttons: Buttons | None = None,
        quick_replies: list[str] | None = None,
    ) -> Stream:
        """Begin a reply that arrives piece by piece::

            async with conv.stream() as reply:
                async for token in model.generate(prompt):
                    await reply.append(token)

        Buttons and quick replies are attached when the stream finishes.
        """
        return self._writable().stream(
            self.id, reply_to=reply_to, buttons=buttons, quick_replies=quick_replies
        )

    @asynccontextmanager
    async def working(self, label: str | None = None) -> AsyncIterator[None]:
        """Keep the person told what you are doing while the block runs::

            async with conv.working("Checking the weather"):
                forecast = await weather(city)

        The label shows under the agent's name, in the chat and in the chat
        list — one line, at most 40 characters, no links. With no label the
        person sees "thinking…". It is renewed while the block runs and
        cleared when it ends, however it ends. If the person presses stop,
        the handler's task is cancelled and the block ends with it.
        """
        agent = self._writable()
        state = "working" if label else "thinking"

        async def say() -> None:
            # Saying what you are doing is a courtesy, never a reason to fail.
            with suppress(Exception):
                await agent.activity(self.id, state, label)

        async def renew() -> None:
            while True:
                await asyncio.sleep(_RENEW_SECONDS)
                await say()

        await say()
        renewing = asyncio.create_task(renew())
        try:
            yield
        finally:
            renewing.cancel()
            # After a stop the hub has already cleared it on every screen.
            if not self.stopped:
                with suppress(Exception):
                    await agent.activity(self.id, "idle")

    def thinking(self) -> Any:
        """``working`` with nothing to name: the person sees "thinking…"."""
        return self.working(None)

    async def activity(self, state: str, label: str | None = None) -> None:
        """Say once what you are doing — ``thinking``, ``working`` with a
        label, or ``idle``. It shows for ten seconds."""
        await self._writable().activity(self.id, state, label)

    async def typing(self, state: str = "start") -> None:
        """Show, or hide, the "working" indicator. The first version of
        ``activity``; a stream shows it by itself."""
        await self._writable().typing(self.id, state)

    def _attached(self) -> Agent:
        if self._agent is None:
            raise RuntimeError("this conversation is not attached to an agent")
        return self._agent

    def _writable(self) -> Agent:
        if self.stopped:
            raise StoppedError()
        return self._attached()
