---
title: Your own agent
description: Create an agent in the app and attach your own harness to it.
---

This is the path for a person who already has something that can hold a
conversation, and wants it reachable from a phone. You create the agent in the
app, and attach whatever you run at home to it.

Nobody else can add your agent unless you mint a code and give it to them, so
you can leave it half-built without consequence.

## Create it in the app

The **Agents** tab has a button to create an agent. You give it three things.

| Field | Rule |
| --- | --- |
| Display name | 1 to 80 characters. Change it whenever you like. |
| Handle | 3 to 32 characters, lower case, digits, `-` and `_`. Permanent. |
| Description | Up to 500 characters. Optional, and editable. |

A picture and up to four starters can be added later from the profile.

A chat with the new agent appears in your list right away, so you have
somewhere to test before anything is connected.

## Attach your harness

Open the agent's profile and choose **Connect**.

**Socket mode** is the default and the one to use. The app shows a secret
starting with `bnd_sec_` and a snippet with your secret and hub already filled
in. Your program dials out, so nothing needs to be reachable from the internet.

**Webhook mode** asks for an HTTPS URL and posts each event to it. Choose it
when your harness is already a web service. You will write the receiver
yourself; see [the protocol reference](/docs/protocol/#webhook-transport) for
the headers and the signature.

The secret is shown once. If you lose it, connect again to get a new one. The
old one stops working the moment the new one is made.

## The smallest harness that works

```python
from cuckoo import Agent

agent = Agent(secret="bnd_sec_...", hub="https://cuckoo.in")


@agent.on_message
async def handle(msg, conv):
    answer = my_own_model(msg.text)      # whatever you already run
    await conv.send(answer)


agent.run()
```

`agent.run()` blocks and reconnects on its own, with a backoff, for as long as
the binding is valid. It gives up and returns in exactly two cases: the binding
was revoked, or another process took it over. Both mean retrying is pointless.

## Wire it to a model

Nothing in the SDK knows about models, so this is ordinary Python. Streaming is
worth the extra three lines, because it turns a long wait into a reply that
arrives as it is written.

```python
async def handle(msg, conv):
    async with conv.stream() as reply:
        async for chunk in my_model.stream(msg.text):
            await reply.append(chunk)
```

Two things to know. A stream that goes quiet for thirty seconds is finished by
the hub and marked truncated, and the app tells the person the reply was cut
short. And the text of one message is capped at 8000 characters; past that the
hub clamps it and marks it truncated too.

## Keeping context

The hub does not remember a conversation for you. It hands you a conversation
id, and history is yours to keep.

```python
history: dict[str, list[str]] = {}


@agent.on_message
async def handle(msg, conv):
    turns = history.setdefault(conv.id, [])
    turns.append(msg.text)
    await conv.send(my_model(turns))
```

Store it by `msg.sender.id` rather than by conversation id if you want it to
follow the person into group chats later. If your process may restart, put it
somewhere that survives that.

Should you need what was said before your code existed, the hub keeps it:
`GET /v1/agent/conversations/{id}/messages` pages back through the chat, as far
as the moment your agent joined.

## Let it speak first

Once someone has added your agent, it may start a conversation. Greet them on
arrival with `@agent.on_join`, or send later when something happens on your
side. An agent cannot message a stranger who has never added it.

## Sharing it

Everything above is private to you. When you want somebody else to have it,
open **Share** on the agent's profile for a QR code and a link, or mint codes
from a script as described in [For companies](/docs/for-companies/). Stop
sharing at any time, and existing chats keep working.

People who add your agent see your name and the label **Unverified**, because
verification does not exist yet.
