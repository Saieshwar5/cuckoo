---
title: Quickstart
description: From nothing to an agent that answers you on your phone, in about ten minutes.
---

At the end of this you will have an agent in your own chat list, answering from
a Python file on your machine. It takes about ten minutes and needs no card,
no cloud account and no model.

You need Python 3.11 or newer, and the Cuckoo app on your phone.

## 1. Install the SDK

The package is not on PyPI yet, so install it from a checkout.

```bash
git clone https://github.com/Saieshwar5/cuckoo.git
cd cuckoo
python -m venv .venv && . .venv/bin/activate
pip install -e sdk/python
```

## 2. Create the agent

Open the app, go to the **Agents** tab, and tap the button to create an agent.
Give it a name. The handle is suggested from the name and you can edit it, but
it is permanent once saved.

## 3. Connect a backend

On the agent's profile, open **Connect** and keep the default, which is socket
mode. The screen shows a binding secret starting with `bnd_sec_`. Copy it now.
It is shown once and never again.

Leave that screen open. It says "Waiting for your backend…" and will change by
itself in a moment.

## 4. Write the agent

Save this as `echo.py`, with your own secret and hub address.

```python
from cuckoo import Agent

agent = Agent(secret="bnd_sec_...", hub="https://cuckoo.in")


@agent.on_message
async def handle(msg, conv):
    await conv.send(f"You said: {msg.text}")


agent.run()
```

Then run it.

```bash
python echo.py
```

The app's Connect screen turns to **Connected** while you are watching.

:::caution
Always pass `hub=` explicitly. The SDK's two entry points ship with different
defaults, so leaving it out is the most common way to spend twenty minutes on
nothing.
:::

## 5. Say something

Open the chat with your agent and send a message. The reply comes back
immediately.

That is the whole loop. Everything else in these docs is a refinement of it.

## Next: make it feel alive

Three small changes cover most of what an agent needs.

**Stream the reply**, so it appears word by word instead of all at once.

```python
@agent.on_message
async def handle(msg, conv):
    async with conv.stream() as reply:
        for word in "Let me think about that for a moment.".split():
            await reply.append(word + " ")
```

**Ask instead of guessing**, with buttons. A tap comes back as an ordinary
message carrying `msg.action`.

```python
@agent.on_message
async def handle(msg, conv):
    if msg.action:
        await conv.send(f"You chose {msg.action.button_id}.")
        return
    await conv.send(
        "Which account do you mean?",
        buttons=[[("salary", "Salary account", "primary"), ("savings", "Savings")]],
        quick_replies=["Neither", "Not sure"],
    )
```

**Greet people when they arrive**, before they type anything.

```python
@agent.on_join
async def greet(conv, token):
    await conv.send("Hello. Ask me anything about your order.")
```

## Where to go next

- [Concepts](/docs/concepts/) explains agent, binding, code and conversation.
- [For companies](/docs/for-companies/) does all of this from a server instead
  of a phone, with an API key.
- [Examples](/docs/examples/) has four runnable agents, including this one.
- [Python SDK](/docs/sdk-python/) is the full reference.
