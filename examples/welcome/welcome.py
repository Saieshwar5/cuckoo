"""The greeter: the first chat a new account has.

The hub adds this agent to every new account when CUCKOO_WELCOME_HANDLE
names its handle, and tells it someone joined. It says hello, explains the
two ways an agent comes to be, and answers a tap. It is an agent like any
other — mute it, remove it, block it — which is the point of building the
first-run screen out of the protocol instead of around it.

    export CUCKOO_SECRET=bnd_sec_...      # from setting a socket binding
    export CUCKOO_HUB=http://localhost:8080
    python welcome.py
"""

import logging
import os

from cuckoo import Agent

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("welcome")

agent = Agent(
    secret=os.environ["CUCKOO_SECRET"],
    hub=os.environ.get("CUCKOO_HUB", "http://localhost:8080"),
)

HELLO = (
    "Hello, and welcome to Cuckoo.\n\n"
    "This is a chat app where the contacts are **agents** — a company's, "
    "a friend's, or your own. Two ways to get one:\n"
    "- **Scan a code** on a company's website or poster, and its agent appears here.\n"
    "- **Create your own** on the Agents tab, then connect your code to it with a "
    "few lines of Python.\n\n"
    "What would you like to know?"
)

ANSWERS = {
    "scan": (
        "Tap **Scan a code** on the Chats tab, or point your camera at a Cuckoo QR "
        "code. You see who is behind the agent before you add it, and you can mute, "
        "remove or block it any time."
    ),
    "create": (
        "On the Agents tab, tap **+**, give it a name, then **Connect**. Paste the "
        "secret into `cuckoo.Agent(secret=...)` in a Python file and run it — from a "
        "laptop is fine, no server needed. Your agent answers from there."
    ),
    "what": (
        "Cuckoo moves messages between people and agents, and never thinks for "
        "either. An agent's replies come from whoever runs it: a company's servers, "
        "or a script on your laptop. The protocol is open, so anyone can build one."
    ),
}

BUTTONS = [[("scan", "Scanning a code"), ("create", "Making my own")], [("what", "What is this?")]]


@agent.on_join
async def greet(conv, token):
    log.info("greeting %s", conv.id)
    await conv.send(HELLO, buttons=BUTTONS)


@agent.on_message
async def handle(msg, conv):
    if msg.action and msg.action.button_id in ANSWERS:
        await conv.send(ANSWERS[msg.action.button_id], buttons=BUTTONS)
        return
    text = msg.text.lower()
    for key in ANSWERS:
        if key in text:
            await conv.send(ANSWERS[key], buttons=BUTTONS)
            return
    await conv.send(
        "I only know about getting started. Ask me about scanning a code, "
        "making your own agent, or what Cuckoo is.",
        buttons=BUTTONS,
    )


if __name__ == "__main__":
    agent.run()
