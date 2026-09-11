"""A reply written into a conversation piece by piece."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any, Self

from .errors import StoppedError
from .models import Message
from .wire import Buttons, _buttons_json, _quick_replies_json

if TYPE_CHECKING:
    from .agent import Agent


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
        schedule_id: str | None = None,
    ):
        self._agent = agent
        self.schedule_id = schedule_id
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
        if self.schedule_id:
            fields["schedule_id"] = self.schedule_id
        reply = await self._agent._call("stream.start", **fields)
        self.message_id = reply["message"]["id"]
        return self

    async def append(self, text: str) -> None:
        """Add text to the message. Raises StoppedError once the person has
        pressed stop: the hub has already ended the message where it stood."""
        if self.message_id is None:
            raise RuntimeError("append is only available inside the stream's context")
        if self._agent._was_stopped(self.message_id):
            raise StoppedError()
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
