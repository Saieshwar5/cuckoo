"""The management client: create agents and hand them out, from a server.

An ``Agent`` speaks for one bot. This speaks for the account that owns
them, with an API key, and does what a person would otherwise do in the
app: create agents, connect their backends, mint the codes that put an
agent in someone's chat list.

    from cuckoo import Management

    cuckoo = Management(key="mgt_tok_...", hub="https://hub.example.com")
    agent = cuckoo.create_agent("sbi-cards", "SBI Cards")
    secret = cuckoo.connect(agent.id)                 # for your backend
    code = cuckoo.create_code(agent.id, payload={"customer_ref": "SBI-8812"})
    print(code.url)                                   # show this as a QR

It is synchronous on purpose: this is deploy-script and web-request code,
not the event loop an agent runs in.
"""

from __future__ import annotations

import base64
import mimetypes
import os
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Self

import httpx

from .agent import ProtocolError
from .models import DEFAULT_HUB

_TIMEOUT = 30.0


@dataclass
class AgentInfo:
    """An agent as its owner sees it. Not the running Agent: that is what a
    backend uses to speak for one."""

    id: str
    handle: str
    display_name: str
    description: str
    # The backend's connection state: idle, connected, unreachable, or None
    # when no backend is attached.
    status: str | None = None
    # Whether it is published with a picture, served at /a/<id>/avatar.
    has_avatar: bool = False
    # What an empty chat suggests saying first; up to four.
    starters: tuple[str, ...] = ()
    # Its backend holds schedules people make; the app offers them only then.
    supports_schedules: bool = False

    @classmethod
    def from_wire(cls, data: dict[str, Any]) -> AgentInfo:
        binding = data.get("binding") or {}
        return cls(
            id=data["id"],
            handle=data.get("handle", ""),
            display_name=data.get("display_name", ""),
            description=data.get("description", ""),
            status=binding.get("status"),
            has_avatar=bool(data.get("has_avatar")),
            starters=tuple(data.get("starters") or ()),
            supports_schedules=bool(data.get("supports_schedules")),
        )


@dataclass
class Code:
    """A pair token: what a QR code or a link contains.

    ``code`` is the secret itself and is shown once. ``url`` is the link to
    put behind a QR; ``qr_png`` is that picture already drawn, as a data
    URL, so a website can show it without a QR library.
    """

    id: str
    code: str
    url: str
    qr_png: str
    max_uses: int | None = None
    use_count: int = 0

    def png_bytes(self) -> bytes:
        """The QR picture as PNG bytes, for writing to a file."""
        return base64.b64decode(self.qr_png.split(",", 1)[1])


class Management:
    """The account's own systems, talking to the hub."""

    def __init__(self, key: str, hub: str = DEFAULT_HUB, *, timeout: float = _TIMEOUT):
        if not key:
            raise ValueError("an API key is required; create one in the app")
        self._hub = hub.rstrip("/")
        self._client = httpx.Client(
            base_url=self._hub,
            headers={"Authorization": f"Bearer {key}"},
            timeout=timeout,
        )

    # -- agents -----------------------------------------------------------

    def upload_avatar(self, path: str | os.PathLike[str]) -> str:
        """Put a picture on the hub, ready to be an agent's face, and return
        its id. Uploaded as the owner, because that is who publishes it."""
        file = Path(path)
        content_type = mimetypes.guess_type(file.name)[0] or "application/octet-stream"
        response = self._client.post(
            "/v1/mgmt/media",
            files={"file": (file.name, file.read_bytes(), content_type)},
            timeout=None,
        )
        _raise_for_status(response)
        return str(response.json()["media"]["id"])

    def create_agent(
        self,
        handle: str,
        display_name: str,
        description: str = "",
        *,
        avatar: str | os.PathLike[str] | None = None,
        starters: list[str] | None = None,
        supports_schedules: bool = False,
    ) -> AgentInfo:
        """Create an agent. The handle is its permanent address on this hub.

        ``avatar`` is a path to the picture it is published with — the logo a
        stranger sees on the card a QR code opens, before they have an
        account. It is uploaded first and named here.

        ``starters`` are up to four short things the empty chat suggests
        saying first — "Track my order", "Talk to a person". Tapping one
        sends its words, so your backend sees plain text.
        """
        body: dict[str, Any] = {
            "handle": handle,
            "display_name": display_name,
            "description": description,
        }
        if avatar is not None:
            body["avatar_media_id"] = self.upload_avatar(avatar)
        if starters is not None:
            body["starters"] = list(starters)
        if supports_schedules:
            body["supports_schedules"] = True
        data = self._call("POST", "/v1/mgmt/agents", body)
        return AgentInfo.from_wire(data["agent"])

    def agents(self) -> list[AgentInfo]:
        """Every agent the account owns."""
        data = self._call("GET", "/v1/mgmt/agents")
        return [AgentInfo.from_wire(a) for a in data["agents"]]

    def agent(self, agent_id: str) -> AgentInfo:
        data = self._call("GET", f"/v1/mgmt/agents/{agent_id}")
        return AgentInfo.from_wire(data["agent"])

    def update_agent(
        self,
        agent_id: str,
        *,
        display_name: str | None = None,
        description: str | None = None,
        avatar: str | os.PathLike[str] | None = None,
        starters: list[str] | None = None,
        supports_schedules: bool | None = None,
    ) -> AgentInfo:
        body: dict[str, Any] = {}
        if supports_schedules is not None:
            body["supports_schedules"] = supports_schedules
        if avatar is not None:
            body["avatar_media_id"] = self.upload_avatar(avatar)
        if starters is not None:
            body["starters"] = list(starters)
        if display_name is not None:
            body["display_name"] = display_name
        if description is not None:
            body["description"] = description
        data = self._call("PATCH", f"/v1/mgmt/agents/{agent_id}", body)
        return AgentInfo.from_wire(data["agent"])

    def delete_agent(self, agent_id: str) -> None:
        """Retire an agent. Its chats stay readable; its handle is not reissued."""
        self._call("DELETE", f"/v1/mgmt/agents/{agent_id}")

    # -- backends ---------------------------------------------------------

    def connect(self, agent_id: str, *, webhook_url: str | None = None) -> str:
        """Attach a backend and return its secret, which is shown once.

        Without a URL the binding is a socket: your code connects out to the
        hub, which works from anywhere. With one, the hub posts to you.
        Connecting again replaces the old binding and cuts off its secret.
        """
        body = (
            {"mode": "webhook", "webhook_url": webhook_url} if webhook_url else {"mode": "socket"}
        )
        return self._call("POST", f"/v1/mgmt/agents/{agent_id}/binding", body)["secret"]

    def disconnect(self, agent_id: str) -> None:
        """Revoke the agent's backend. It goes quiet; its history stays."""
        self._call("DELETE", f"/v1/mgmt/agents/{agent_id}/binding")

    # -- handing an agent out ---------------------------------------------

    def create_code(
        self,
        agent_id: str,
        *,
        payload: Any = None,
        max_uses: int | None = None,
        expires_in: int | None = None,
    ) -> Code:
        """Mint a code that puts this agent in someone's chat list.

        Leave everything out for a poster: any number of people, no expiry.
        Pass ``max_uses=1`` with your own reference in ``payload`` for a code
        minted per customer — the payload comes back to your backend in the
        ``conversation.joined`` event, before they say a word.
        """
        body: dict[str, Any] = {}
        if payload is not None:
            body["payload"] = payload
        if max_uses is not None:
            body["max_uses"] = max_uses
        if expires_in is not None:
            body["expires_in"] = expires_in
        data = self._call("POST", f"/v1/mgmt/agents/{agent_id}/pair-tokens", body)
        token = data["token"]
        return Code(
            id=token["id"],
            code=data["code"],
            url=data["url"],
            qr_png=data["qr_png"],
            max_uses=token.get("max_uses"),
            use_count=token.get("use_count", 0),
        )

    def codes(self, agent_id: str) -> list[dict[str, Any]]:
        """The agent's codes, newest first, revoked ones included.

        The secrets are not here: the hub keeps only their hashes, so a code
        exists in full exactly once, in the answer that minted it.
        """
        return self._call("GET", f"/v1/mgmt/agents/{agent_id}/pair-tokens")["tokens"]

    def revoke_code(self, agent_id: str, token_id: str) -> None:
        """Stop a code working. People who already added the agent keep it."""
        self._call("DELETE", f"/v1/mgmt/agents/{agent_id}/pair-tokens/{token_id}")

    # -- plumbing ---------------------------------------------------------

    def close(self) -> None:
        self._client.close()

    def __enter__(self) -> Self:
        return self

    def __exit__(self, *_: object) -> None:
        self.close()

    def _call(self, method: str, path: str, body: dict[str, Any] | None = None) -> dict[str, Any]:
        response = self._client.request(method, path, json=body)
        _raise_for_status(response)
        if response.status_code == 204 or not response.content:
            return {}
        return response.json()


def _raise_for_status(response: httpx.Response) -> None:
    """Turn a refusal into the hub's own error, so a caller sees
    "unknown_picture" rather than "422 Unprocessable Entity"."""
    if response.status_code < 400:
        return
    try:
        error = response.json()["error"]
    except Exception:  # noqa: BLE001 - a hub that answered with prose
        raise ProtocolError(
            "http_error", f"{response.status_code}: {response.text[:200]}"
        ) from None
    raise ProtocolError(error.get("code", "error"), error.get("message", ""))
