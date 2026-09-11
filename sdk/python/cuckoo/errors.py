"""The one error shape the hub answers with, and how a refusal becomes it."""

from __future__ import annotations

import httpx


class ProtocolError(Exception):
    """The hub refused an operation. ``code`` is the stable error code."""

    def __init__(self, code: str, message: str):
        super().__init__(f"{code}: {message}")
        self.code = code
        self.message = message


class StoppedError(ProtocolError):
    """The person pressed stop.

    Raised by a write into a reply they stopped, and by any write from a
    handler whose conversation they stopped. It is not a failure to report:
    a handler that lets it escape is treated as finished — acknowledged,
    never retried — because doing the work again is exactly what the person
    asked not to happen.
    """

    def __init__(self, message: str = "The person stopped this reply."):
        super().__init__("stopped", message)


def error_of(code: str, message: str) -> ProtocolError:
    """The right exception for a code the hub sent."""
    if code == "stopped":
        return StoppedError(message)
    return ProtocolError(code, message)


def raise_for_status(response: httpx.Response) -> None:
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
    raise error_of(error.get("code", "error"), error.get("message", ""))
