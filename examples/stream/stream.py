"""An agent that answers a word at a time, the way a model does.

    export CUCKOO_SECRET=bnd_sec_...      # from setting a socket binding
    export CUCKOO_HUB=http://localhost:8080
    python stream.py

Set STREAM_DELAY to change the pause between words (seconds).
"""

import asyncio
import logging
import os

from cuckoo import Agent

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("stream")

agent = Agent(
    secret=os.environ["CUCKOO_SECRET"],
    hub=os.environ.get("CUCKOO_HUB", "http://localhost:8080"),
)
delay = float(os.environ.get("STREAM_DELAY", "0.15"))


@agent.on_message
async def handle(msg, conv):
    log.info("%s said: %s", msg.sender.display_name, msg.text)
    async with conv.stream() as reply:
        for word in f"You said: {msg.text}. I am typing this out slowly.".split():
            await reply.append(word + " ")
            await asyncio.sleep(delay)
    log.info("finished %s", reply.message.id if reply.message else "?")


if __name__ == "__main__":
    agent.run()
