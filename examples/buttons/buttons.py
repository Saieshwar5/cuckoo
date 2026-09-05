"""An agent that asks before it acts.

It greets whoever adds it, by the reference in the code they scanned when
there is one. It answers a question with two buttons, and acts on the tap:
the choice comes back as an id it can match, never as text it has to
interpret.

    export CUCKOO_SECRET=bnd_sec_...      # from setting a socket binding
    export CUCKOO_HUB=http://localhost:8080
    python buttons.py
"""

import logging
import os

from cuckoo import Agent

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("buttons")

agent = Agent(
    secret=os.environ["CUCKOO_SECRET"],
    hub=os.environ.get("CUCKOO_HUB", "http://localhost:8080"),
)

ACCOUNTS = {"acc-salary": "the salary account", "acc-savings": "the savings account"}


@agent.on_join
async def greet(conv, token):
    # A personalised code carries the bank's own reference; a poster carries
    # nothing, and the agent asks.
    ref = (token.payload or {}).get("customer_ref") if token else None
    if ref:
        await conv.send(
            f"Hello! I can see your account ending {ref[-4:]}. What can I do for you?"
        )
    else:
        await conv.send("Hello! Which account is this about?")


@agent.on_message
async def handle(msg, conv):
    if msg.action:
        chosen = ACCOUNTS.get(msg.action.button_id, "an account I do not know")
        log.info("%s chose %s", msg.sender.display_name, msg.action.button_id)
        await conv.send(f"Checking {chosen} now.", reply_to=msg.id)
        return

    log.info("%s said: %s", msg.sender.display_name, msg.text)
    await conv.typing()
    await conv.send(
        "Which account do you mean?",
        buttons=[
            [("acc-salary", "Salary account", "primary"), ("acc-savings", "Savings")]
        ],
        quick_replies=["Neither", "Not sure"],
        reply_to=msg.id,
    )


if __name__ == "__main__":
    agent.run()
