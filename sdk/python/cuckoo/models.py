"""What a handler is given: the message, and the conversation to reply into."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from .agent import Agent, Buttons, Stream


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
class Message:
    """One thing said in a conversation. ``action`` is set when the person
    tapped one of your buttons; ``text`` is then the button's label."""

    id: str
    conversation_id: str
    text: str
    sender: Sender
    created_at: str
    status: str = "complete"
    truncated: bool = False
    action: Action | None = None
    reply_to: ReplyRef | None = None
    event_id: str | None = None

    @classmethod
    def from_wire(
        cls, data: dict[str, Any], conversation_id: str, event_id: str | None = None
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
            action=Action(action["button_id"], action["source_message_id"]) if action else None,
            reply_to=ReplyRef(
                reply["id"], reply.get("sender_kind", ""), reply.get("text_preview", "")
            )
            if reply
            else None,
            event_id=event_id,
        )


@dataclass
class Conversation:
    """Where a message was said, and the handle for replying there."""

    id: str
    kind: str
    participants: list[Participant] = field(default_factory=list)
    _agent: Agent | None = field(default=None, repr=False, compare=False)

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
        text: str,
        *,
        buttons: Buttons | None = None,
        quick_replies: list[str] | None = None,
        reply_to: str | None = None,
        idempotency_key: str | None = None,
    ) -> Message:
        """Say something in this conversation.

        ``buttons`` is rows of ``(id, label)`` or ``(id, label, style)``; the
        person's tap comes back as a message whose ``action.button_id`` is
        the id. ``quick_replies`` are suggested answers sent as plain text.
        """
        return await self._attached().send(
            self.id,
            text,
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
        return self._attached().stream(
            self.id, reply_to=reply_to, buttons=buttons, quick_replies=quick_replies
        )

    async def typing(self, state: str = "start") -> None:
        """Show, or hide, the "working" indicator. A stream shows it by itself."""
        await self._attached().typing(self.id, state)

    def _attached(self) -> Agent:
        if self._agent is None:
            raise RuntimeError("this conversation is not attached to an agent")
        return self._agent
