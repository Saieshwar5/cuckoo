# cuckoo-agent

The Python SDK for the Cuckoo Agent Protocol. Connect a backend to a hub,
receive messages, reply.

```python
from cuckoo import Agent

agent = Agent(secret="bnd_sec_...", hub="http://localhost:8080")


@agent.on_message
async def handle(msg, conv):
    await conv.send(f"You said: {msg.text}")


agent.run()
```

Every message the hub hands you carries `msg.signature`, the hub's own seal
over what was said; the hub keeps messages for 90 days, so if you keep them
longer, keep the signature with them, unchanged.

Photos and files arrive as attachments, and the bytes are fetched only if
something wants them:

```python
@agent.on_message
async def handle(msg, conv):
    for file in msg.attachments:
        if file.is_image:
            path = await file.save("/tmp")           # or: await file.download()
            await conv.send("Got your photo.", attachments=[path])
```

An agent can be published with its logo, which is what a stranger sees on
the card a QR code opens, and with a few starters — what an empty chat
suggests saying first, so nobody stares at a blank screen:

```python
cuckoo.create_agent("sbi-cards", "SBI Cards", avatar="logo.png",
                    starters=["Card blocked", "Statement", "Talk to a person"])
cuckoo.update_agent(agent.id, avatar="newlogo.png")
```

Words are markdown-lite — bold, italic, code, bullet and numbered lists,
and addresses that become links. A buttons row may carry a `Link`, which
opens something on the person's phone and tells you nothing:

```python
from cuckoo import Link

await conv.send(
    "**Order 4412** is on its way:\n- Rider: Ravi\n- ETA: 12 min",
    buttons=[[Link("Track order", "https://swiggy.com/t/4412"), ("cancel", "Cancel")]],
)
```

`[label](url)` is not a link and is shown as written: a worded link in text
would let any agent dress up an address as somewhere else. A `Link` button
is the worded link, and the app draws it as one that leaves.

A voice note arrives with what it needs to be drawn and heard:

```python
if file.is_audio:
    print(file.seconds, file.waveform)   # 8.2, (3, 40, 88, ...)
    speech = await file.download()
```

and goes back the same way, since nothing downstream can work either out
from the bytes:

```python
note = await agent.upload("reply.m4a", duration_ms=4100, waveform=bars, audio=True)
await conv.send(attachments=[note])
```

`conv.send` uploads anything in `attachments` that is a path, then sends one
message naming what was uploaded — so a message carrying files needs no text,
and the text, when there is any, is their caption.

What it does for you:

- Holds a socket to the hub and reconnects with backoff when it drops.
- Receives every message the agent missed while away, in order.
- Calls your handler once per message, and acknowledges the event only after
  the handler returns. A handler that raises leaves the event unacknowledged,
  and the hub sends it again.
- Sends replies with an idempotency key, so a retried reply never doubles.
- Uploads and downloads files, off the event loop, so one big photo does not
  stop the other conversations.
- Stops, rather than fights, when a newer connection takes over the binding
  or the binding is revoked.

Install from the repository for now:

```bash
pip install -e sdk/python
```
