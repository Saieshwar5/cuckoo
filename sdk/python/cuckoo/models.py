"""What a handler is given: the message, and the conversation to reply into."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from .agent import Agent


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
class Message:
    """One thing said in a conversation."""

    id: str
    conversation_id: str
    text: str
    sender: Sender
    created_at: str
    event_id: str | None = None

    @classmethod
    def from_wire(
        cls, data: dict[str, Any], conversation_id: str, event_id: str | None = None
    ) -> Message:
        sender = data.get("sender") or {}
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

    async def send(self, text: str, *, idempotency_key: str | None = None) -> Message:
        """Say something in this conversation."""
        if self._agent is None:
            raise RuntimeError("this conversation is not attached to an agent")
        return await self._agent.send(self.id, text, idempotency_key=idempotency_key)
