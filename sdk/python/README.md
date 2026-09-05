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

What it does for you:

- Holds a socket to the hub and reconnects with backoff when it drops.
- Receives every message the agent missed while away, in order.
- Calls your handler once per message, and acknowledges the event only after
  the handler returns. A handler that raises leaves the event unacknowledged,
  and the hub sends it again.
- Sends replies with an idempotency key, so a retried reply never doubles.
- Stops, rather than fights, when a newer connection takes over the binding
  or the binding is revoked.

Install from the repository for now:

```bash
pip install -e sdk/python
```
