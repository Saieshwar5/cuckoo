---
title: For companies
description: Get an API key, create an agent from your server, connect a backend, and hand it out by QR code.
---

This is the path where a company runs the agent and strangers add it. One agent
serves every customer. Each person who adds it gets a private conversation, and
your backend keeps its own state per person.

Nothing here needs a phone except the first step, and that is being lifted.

## 1. Get an API key

An API key belongs to a person's account on the hub. An engineer signs in with
their email address, opens **Settings**, then **API keys**, and creates one.

The key starts with `mgt_tok_` and is shown once. Put it in your secret store.
It can be revoked from the same screen, and revoking it stops every script
using it immediately.

A key can create and manage agents. It cannot mint another key, and it cannot
read anybody's messages.

:::note
Needing the app for this one step is a known wrinkle. A `cuckoo login` command
that mints the first key from a terminal is being built.
:::

## 2. Create the agent

Install the SDK, then run this once, ever.

```python
from cuckoo import Management

cuckoo = Management(key="mgt_tok_...", hub="https://cuckoo.in")

agent = cuckoo.create_agent(
    "sbi-cards",
    "SBI Cards",
    description="Block a card, get a statement, raise a dispute.",
    avatar="logo.png",
    starters=["Card blocked", "Statement", "Talk to a person"],
)
print(agent.id)
```

| Field | Rule |
| --- | --- |
| handle | 3 to 32 characters, lower case, digits, `-` and `_`. Permanent. Some words are reserved. |
| display name | 1 to 80 characters. |
| description | Up to 500 characters. |
| starters | Up to four, 40 characters each. Drawn as chips in an empty chat. |
| avatar | An image you upload. It is public, because a stranger sees it before they add anything. |

The same thing from a terminal:

```bash
export CUCKOO_KEY=mgt_tok_...
export CUCKOO_HUB=https://cuckoo.in
cuckoo agents create sbi-cards "SBI Cards" \
  --description "Block a card, get a statement, raise a dispute." \
  --avatar logo.png --starter "Card blocked" --starter "Statement"
```

Create one agent per job, not one per customer. A support agent, an orders
agent and a billing agent are three agents. Ten million customers are ten
million conversations with one agent.

## 3. Connect your backend

```python
secret = cuckoo.connect(agent.id)          # bnd_sec_..., shown once
```

Store that secret the way you store a database password. Then run your backend
against it, as an ordinary long-lived process.

```python
from cuckoo import Agent

agent = Agent(secret=os.environ["CUCKOO_SECRET"], hub="https://cuckoo.in")


@agent.on_message
async def handle(msg, conv):
    await conv.send(answer_for(conv.id, msg.text))


agent.run()
```

Socket mode is the default and needs no inbound port. If you would rather the
hub posted to you, pass `webhook_url=` instead and write the receiver against
[the protocol reference](/docs/protocol/#webhook-transport). The Python SDK has
no webhook receiver, so socket mode is much less work.

Calling `connect` again replaces the binding and invalidates the old secret.
That is the rotation path, and the vendor-swap path. Only one process may hold
a socket at a time: a second connection takes over and the first is closed.

## 4. Hand the agent out

A code is a link and a QR image. Mint as many as you need.

**A poster code** has no limit and no expiry. Print it, put it on a statement,
or link it from your site.

```python
code = cuckoo.create_code(agent.id)
print(code.url)                         # https://cuckoo.in/p/pair_...
open("poster.png", "wb").write(code.png_bytes())
```

**A personalised code** carries a payload you choose, and is usually good for
one use. Mint it when a logged-in customer asks to chat, and your backend is
told who they are before they type.

```python
code = cuckoo.create_code(
    agent.id,
    payload={"customer_ref": "SBI-8812"},
    max_uses=1,
    expires_in=600,                     # ten minutes is plenty
)
```

Or from a terminal, which is enough for a poster:

```bash
cuckoo codes create <agent id> --qr poster.png
cuckoo codes list <agent id>
cuckoo codes revoke <agent id> <token id>
```

The code itself is returned once, like every other secret here. The hub keeps
only its hash, so if you want the QR again later you must supply the code you
saved.

## 5. Know who is talking

Read [Identity](/docs/identity/). It is the page that decides how your
integration feels, and it is short.

The short version: store a link between the hub's `sender.id` and your own
customer reference the first time you learn it, either from the payload on a
personalised code or by asking in chat. Every later message is then recognised.

## Managing it afterwards

| Task | How |
| --- | --- |
| Change name, description, logo or starters | `cuckoo.update_agent(agent.id, display_name="…")` |
| Take the agent offline | `cuckoo.disconnect(agent.id)`. It goes quiet, history stays, the app tells people it is away. |
| Rotate the secret | `cuckoo.connect(agent.id)` again. |
| Stop new people adding it | `cuckoo.revoke_code(agent.id, token_id)`. Existing chats are untouched. |
| See what is live | `cuckoo.agent(agent.id).status` is `idle`, `connected` or `unreachable`. |
| Remove it entirely | `cuckoo.delete_agent(agent.id)`. The handle is never reissued. |

## What your customers can do

Plan for all of it, because it will happen.

- **Mute** your agent for eight hours, a week, or forever.
- **Block** it. Your backend gets a `conversation.left` event and sends after
  that are refused. Stop trying.
- **Remove** it from their list, or **report** it.
- **Delete their account**, which retires the conversation.

Your agent shows your account's name and an **Unverified** label to everyone
who sees its card. Domain verification is not built yet.

## Things that will bite you

- **A message is capped at 8000 characters.** Long answers need splitting, or
  a stream.
- **Sends are limited to 60 a second per agent**, in bursts of 120. That is per
  agent, not per conversation, so a broadcast to a hundred thousand people
  needs pacing on your side.
- **A stream that goes quiet for thirty seconds is closed** by the hub and
  marked truncated.
- **A backend that is down loses nothing.** The hub retries each event on a
  ladder from one second out to an hour, and gives up after 24 hours. When you
  reconnect, everything unacknowledged is delivered again, so handlers must
  tolerate seeing an event twice.
- **A join is not always personalised.** If somebody blocks your agent and adds
  it again, the join arrives with no payload. Do not assume it is there.
