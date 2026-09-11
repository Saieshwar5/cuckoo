"""How buttons, quick replies and attachments are written on the wire."""

from __future__ import annotations

import os

from .models import Attachment, Link

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
