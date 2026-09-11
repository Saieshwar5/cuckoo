"""Schedules: things a person asked the agent to do at a time.

Your backend runs them — the timer, the work, the answer. The hub shows
them in the app and passes on what the person does to them. Nothing fires on
the hub's side, so a schedule your backend is not holding does nothing.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any

from .errors import raise_for_status

if TYPE_CHECKING:
    from .agent import Agent


@dataclass(frozen=True)
class Schedule:
    """One schedule. ``cadence`` is structure, never a sentence::

        {"repeat": "daily", "time": "07:00", "timezone": "Asia/Kolkata"}

    ``repeat`` is once, daily, weekdays or weekly; weekly has ``days``
    ("mon" … "sun"), once has a ``date`` ("2026-09-12"). ``status`` is
    pending until you confirm it, and paused only the person undoes.
    ``next_run_at`` is worked out by the hub; ``missed_at`` is the last time
    it was due and nothing was sent for it.
    """

    id: str
    conversation_id: str
    title: str
    instruction: str
    cadence: dict[str, Any]
    status: str
    created_by: str
    next_run_at: str | None = None
    last_run_at: str | None = None
    missed_at: str | None = None

    @classmethod
    def from_wire(cls, data: dict[str, Any]) -> Schedule:
        return cls(
            id=data.get("id", ""),
            conversation_id=data.get("conversation_id", ""),
            title=data.get("title", ""),
            instruction=data.get("instruction", ""),
            cadence=dict(data.get("cadence") or {}),
            status=data.get("status", "pending"),
            created_by=data.get("created_by", "user"),
            next_run_at=data.get("next_run_at"),
            last_run_at=data.get("last_run_at"),
            missed_at=data.get("missed_at"),
        )


@dataclass(frozen=True)
class ScheduleChange:
    """What the person did in the app: ``requested`` (hold it, then confirm),
    ``updated`` (paused, resumed, or a new time or instruction — pending
    until you confirm again), or ``deleted`` (stop its timer)."""

    type: str
    schedule: Schedule


@dataclass
class Schedules:
    """The schedules in one conversation, from your side: ``conv.schedules``."""

    conversation_id: str
    _agent: Agent | None = field(default=None, repr=False)

    async def list(self) -> list[Schedule]:
        data = await self._request("GET", "")
        return [Schedule.from_wire(s) for s in data.get("schedules") or []]

    async def create(self, *, title: str, instruction: str, cadence: dict[str, Any]) -> Schedule:
        """Record a schedule you already hold — one asked for in the chat —
        so the person sees it with the ones they made in the app."""
        data = await self._request(
            "POST", "", {"title": title, "instruction": instruction, "cadence": cadence}
        )
        return Schedule.from_wire(data["schedule"])

    async def confirm(self, schedule_id: str, title: str | None = None) -> Schedule:
        """Say you hold one the person asked for, and give it a name."""
        body: dict[str, Any] = {"status": "active"}
        if title:
            body["title"] = title
        return await self.update(schedule_id, **body)

    async def update(self, schedule_id: str, **change: Any) -> Schedule:
        """Change ``title``, ``instruction``, ``cadence`` or ``status``."""
        data = await self._request("PATCH", f"/{schedule_id}", change)
        return Schedule.from_wire(data["schedule"])

    async def remove(self, schedule_id: str) -> None:
        """Let go of one: decline a request, or end a one-off that has run."""
        await self._request("DELETE", f"/{schedule_id}")

    async def _request(self, method: str, suffix: str, body: dict[str, Any] | None = None) -> dict[str, Any]:
        agent = self._agent
        if agent is None or agent._http is None:
            raise RuntimeError("schedules are only available while the agent is running")
        response = await agent._http.request(
            method,
            f"/v1/agent/conversations/{self.conversation_id}/schedules{suffix}",
            json=body,
        )
        raise_for_status(response)
        return response.json() if response.content else {}
