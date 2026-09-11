---
title: Examples
description: Four runnable agents, in the order they are worth reading.
---

Four agents live in `examples/` in the repository. Each is a single file with
its own README, and they are worth reading in this order.

All four take the same two environment variables.

```bash
pip install -e sdk/python
CUCKOO_SECRET=bnd_sec_... CUCKOO_HUB=https://cuckoo.onl python echo.py
```

## echo — the smallest thing that works

Repeats what you say, and sends files back. It covers the minimum handler,
reading attachments, saving them, and re-uploading a voice note with its
duration and waveform intact so it comes back as a voice note rather than a
file.

```python
@agent.on_message
async def handle(msg, conv):
    if not msg.has_attachments:
        await conv.send(f"You said: {msg.text}")
        return
    with tempfile.TemporaryDirectory() as tmp:
        sent = []
        for att in msg.attachments:
            path = await att.save(Path(tmp))
            sent.append(await agent.upload(
                path,
                duration_ms=att.duration_ms,
                waveform=list(att.waveform),
                audio=att.is_audio,
            ))
        await conv.send("Here it is again.", attachments=sent)
```

Its README has the smoke test worth running once: stop the script, send three
messages, start it again. All three arrive, in order. That is the outbox doing
its job.

## stream — a word at a time

Types its reply out slowly, so you can see what streaming looks like from the
other side.

```python
@agent.on_message
async def handle(msg, conv):
    async with conv.stream() as reply:
        for word in f"You said: {msg.text}.".split():
            await reply.append(word + " ")
            await asyncio.sleep(delay)
```

Kill it mid-sentence and the hub finishes the message by itself thirty seconds
after the last word, marked truncated. The app says the reply was cut short.

## buttons — asking before acting

The one to copy when your agent needs a decision. It greets a person using the
payload from their code, says it is thinking, offers two styled buttons
and two quick replies, and dispatches on which button was tapped.

```python
@agent.on_join
async def greet(conv, token):
    ref = (token.payload or {}).get("customer_ref") if token else None
    if ref:
        await conv.send(f"Hello. I can see your account ending {ref[-4:]}.")
    else:
        await conv.send("Hello. Which account is this about?")


@agent.on_message
async def handle(msg, conv):
    if msg.action:
        await conv.send(f"Checking {ACCOUNTS[msg.action.button_id]} now.", reply_to=msg.id)
        return
    await conv.typing()
    await conv.send(
        "Which account do you mean?",
        buttons=[[("acc-salary", "Salary account", "primary"), ("acc-savings", "Savings")]],
        quick_replies=["Neither", "Not sure"],
        reply_to=msg.id,
    )
```

## welcome — an agent as a first-run screen

The agent every new account starts with. It shows what an agent can do when it
is used as product furniture rather than a chatbot: multi-row buttons, formatted
text, and one map answering both a tap and free text.

A hub started with `CUCKOO_WELCOME_HANDLE` set adds this agent to every new
account and fires a join event, so a new person's chat list is never empty.

```bash
cuckoo agents create welcome "Cuckoo" \
  --description "Says hello, and how this works." \
  --starter "How do I add an agent?" --starter "How do I make my own?"
cuckoo agents connect <agent id>
CUCKOO_SECRET=bnd_sec_... python welcome.py
```

On a development hub, `make play` does all of that for you.

## What is not here yet

There is no example that calls a language model. Wiring one in is ordinary
Python: take `msg.text`, call your provider, and stream the chunks back with
`reply.append`. See [Your own agent](/docs/your-own-agent/#wire-it-to-a-model).
