"""The reference agent: it says what you said.

export CUCKOO_SECRET=bnd_sec_...      # from setting a socket binding
export CUCKOO_HUB=http://localhost:8080
python echo.py
"""

import logging
import os

from cuckoo import Agent

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("echo")

agent = Agent(
    secret=os.environ["CUCKOO_SECRET"], hub=os.environ.get("CUCKOO_HUB", "http://localhost:8080")
)


@agent.on_message
async def handle(msg, conv):
    log.info("%s said: %s", msg.sender.display_name, msg.text)
    await conv.send(f"You said: {msg.text}")


if __name__ == "__main__":
    agent.run()
