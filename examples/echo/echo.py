"""The reference agent: it says what you said, and sends back what you sent.

export CUCKOO_SECRET=bnd_sec_...      # from setting a socket binding
export CUCKOO_HUB=http://localhost:8080
python echo.py
"""

import logging
import os
import tempfile
from pathlib import Path

from cuckoo import Agent

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("echo")

agent = Agent(
    secret=os.environ["CUCKOO_SECRET"],
    hub=os.environ.get("CUCKOO_HUB", "http://localhost:8080"),
)


@agent.on_message
async def handle(msg, conv):
    log.info("%s said: %s", msg.sender.display_name, msg.text or "(nothing)")

    if not msg.has_attachments:
        await conv.send(f"You said: {msg.text}")
        return

    # Files arrive as ids: the bytes are fetched only if something wants
    # them. Here everything is downloaded and sent straight back, which is
    # the smallest honest proof that both directions work.
    with tempfile.TemporaryDirectory() as tmp:
        paths = []
        for att in msg.attachments:
            path = await att.save(Path(tmp))
            log.info(
                "got %s (%s, %d bytes) -> %s",
                att.file_name,
                att.kind,
                att.byte_size,
                path,
            )
            paths.append(path)
        again = (
            "Here it is again."
            if len(paths) == 1
            else f"Here are your {len(paths)} files again."
        )
        await conv.send(again, attachments=paths)


if __name__ == "__main__":
    agent.run()
