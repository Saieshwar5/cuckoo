---
title: Python SDK
description: Reference for the cuckoo Python package — Agent, Conversation, Message and Management.
---

The Python SDK is two halves. `Agent` runs your backend and is asynchronous.
`Management` creates and manages agents and is deliberately synchronous, for
deploy scripts and web requests.

## Install

Python 3.11 or newer. The package is **not on PyPI yet**, so install it from a
checkout of the repository.

```bash
pip install -e sdk/python
```

It depends on `httpx` and `websockets`, and nothing else.

## Agent

```python
Agent(secret: str, hub: str, *, max_backoff: float = 30.0)
```

`secret` is a binding secret and must start with `bnd_sec_`. `hub` defaults to
the public hub, `https://cuckoo.onl`; pass `http://localhost:8080` for a hub on
your own machine. Forgetting that is the most common first mistake.

### Handlers

All three must be `async def`.

```python
@agent.on_message
async def handle(msg: Message, conv: Conversation) -> None: ...

@agent.on_join
async def greet(conv: Conversation, token: PairToken | None) -> None: ...

@agent.on_stop
async def stopped(stop: StopRequest, conv: Conversation) -> None: ...
```

`on_message` is required. `on_join` is optional and fires when somebody adds
your agent; `token` carries the payload from a personalised code, and is `None`
when there was none. `on_stop` is optional too: see
[When the person presses stop](#when-the-person-presses-stop).

Registering twice replaces the handler rather than adding one.

### Running

```python
agent.run()             # blocking; wraps asyncio.run(agent.serve())
await agent.serve()     # the coroutine, if you have your own loop
```

`serve` reconnects with a backoff that doubles up to `max_backoff` and resets
after a connection that lasted a minute. It gives up and returns in three
cases, all of which mean retrying is pointless: the credential was rejected,
another process took the binding, or the binding was revoked.

### Delivery

Handlers run as separate tasks, so a slow one does not block the socket. An
event is acknowledged **after** your handler returns. A handler that raises
leaves the event unacknowledged, and the hub delivers it again, so make your
handlers safe to run twice. The last thousand event ids are remembered to drop
obvious repeats.

### Errors

```python
class ProtocolError(Exception):
    code: str        # the hub's stable code, e.g. "file_too_large"
    message: str

class StoppedError(ProtocolError): ...   # code "stopped": the person pressed stop
```

## Conversation

| Attribute | Type | Meaning |
| --- | --- | --- |
| `id` | `str` | `cnv_…` |
| `kind` | `str` | `dm` today |
| `participants` | `list[Participant]` | The agent itself has `is_me=True` |

```python
await conv.send(
    text="",
    *,
    attachments=None,        # paths, Attachments, or media ids
    buttons=None,
    quick_replies=None,      # list[str]
    reply_to=None,           # a message id
    idempotency_key=None,    # generated for you if omitted
) -> Message

```

`text` may be empty when there are attachments; then it is their caption.

### Streaming

`conv.stream()` is **not** a coroutine. It returns a context manager.

```python
async with conv.stream() as reply:
    for word in words:
        await reply.append(word + " ")
print(reply.message.id)
```

Buttons and quick replies passed to `stream()` are attached when it ends.
Leaving the block finishes the message even if your code raised.

### Saying what you are doing

```python
async with conv.working("Checking the weather"):
    forecast = await weather(city)

async with conv.thinking():
    plan = await model(prompt)
```

The person sees *Checking the weather…* under the agent's name, in the chat and
in their chat list, and a stop button where send was. The label is one line, at
most 40 characters, no links. It is renewed while the block runs and cleared
when it ends, however it ends. A stream says *writing…* by itself.

### When the person presses stop

You need to write nothing for stop to work. The task running your handler for
that conversation is cancelled, `conv.stopped` becomes true, and any write from
it raises `StoppedError`. Either way the event is acknowledged, not retried:
doing the work again is exactly what the person asked not to happen. A
`try/finally` in your handler still runs, so clean up there.

Say something true afterwards, if there is anything worth saying:

```python
@agent.on_stop
async def stopped(stop, conv):
    await conv.send("Stopped. Nothing was booked.")
```

`stop.message_id` is the reply the hub ended, or `None` if you had not started
writing. The hub has already done its part before this runs: the reply ended
where it stood, marked `stopped`, and the indicator is gone from every screen.

### Schedules

For an agent created with `supports_schedules=True`. Your backend holds the
timer; the app shows the schedule and passes on what the person does.

```python
@agent.on_schedule
async def schedule(change, conv):
    if change.type == "deleted":
        return timers.cancel(change.schedule.id)
    timers.set(change.schedule.id, change.schedule.cadence, change.schedule.instruction)
    await conv.schedules.confirm(change.schedule.id, "Morning weather")

# your timer fires:
await conv.send(forecast, schedule_id=schedule_id)
```

`conv.schedules` also has `list`, `create` (for one made in the chat),
`update` and `remove`. A send for a schedule the person paused or deleted
raises `ProtocolError` with `schedule_paused` or `schedule_deleted`: stop that
timer. See [Schedules](/docs/protocol/#schedules) for the rules.

## Message

| Field | Type | Notes |
| --- | --- | --- |
| `id` | `str` | |
| `conversation_id` | `str` | |
| `text` | `str` | For a button tap, the button's label |
| `sender` | `Sender` | `kind`, `id`, `display_name` |
| `created_at` | `str` | ISO-8601 text, not a `datetime` |
| `status` | `str` | `complete` or `streaming` |
| `truncated` | `bool` | The hub finished a stream: you went quiet, or the person stopped it |
| `stopped` | `bool` | The person pressed stop while it was being written |
| `action` | `Action \| None` | Set when a button of yours was tapped |
| `reply_to` | `ReplyRef \| None` | |
| `signature` | `str \| None` | The hub's seal over the message. Keep it with the message, unchanged. `None` on messages older than signing |
| `attachments` | `tuple[Attachment, ...]` | A tuple, not a list |
| `has_attachments` | `bool` | Property |

`Sender` carries `kind`, `id` and `display_name`. That is everything the SDK
ever learns about a person.

## Buttons

A list of rows, each row a list of buttons. A button may be a tuple, a dict or
a `Link`.

```python
from cuckoo import Link

await conv.send(
    "Which account do you mean?",
    buttons=[
        [("acc-salary", "Salary account", "primary"), ("acc-savings", "Savings")],
        [Link("Open in the app", "https://bank.example/accounts")],
    ],
    quick_replies=["Neither", "Not sure"],
    reply_to=msg.id,
)
```

| Form | Becomes |
| --- | --- |
| `("id", "Label")` | an id button, default style |
| `("id", "Label", "primary")` | an id button with a style |
| `{"id": …, "label": …, "style": …}` | the same |
| `{"url": …, "label": …}` | a url button |
| `Link("Label", "https://…")` | a url button |

A button carries an id or a url, never both. A tap on an id button comes back
as a message with `msg.action.button_id`. A url button opens something on the
device and tells you nothing.

Quick replies are plain strings. Tapping one sends its words as ordinary text.

## Attachments

Sending a file is one argument. Paths are uploaded for you.

```python
await conv.send("Here is the statement.", attachments=["statement.pdf"])
```

Receiving one:

```python
@agent.on_message
async def handle(msg, conv):
    for file in msg.attachments:
        if file.is_image:
            path = await file.save("/tmp")          # a directory, or a file path
            data = await file.download()            # or the bytes directly
```

| Field | Meaning |
| --- | --- |
| `media_id`, `kind`, `mime_type`, `file_name`, `byte_size` | The file |
| `width`, `height`, `has_thumbnail` | Pictures and video |
| `duration_ms`, `waveform`, `seconds` | Recordings |
| `is_image`, `is_audio` | Convenience properties |

Uploading by hand, which you need for a voice note:

```python
note = await agent.upload("reply.m4a", duration_ms=4100, waveform=bars, audio=True)
await conv.send(attachments=[note])
```

Duration and waveform come from the recording device. The hub does not decode
audio, so pass them back when you re-send a note. `audio=True` is what marks a
WebM, Ogg or MP4 container as a recording rather than a video.

## Management

Synchronous, and usable as a context manager.

```python
from cuckoo import Management

with Management(key="mgt_tok_...", hub="https://cuckoo.onl") as cuckoo:
    agent = cuckoo.create_agent("sbi-cards", "SBI Cards", avatar="logo.png")
    secret = cuckoo.connect(agent.id)
```

| Method | Returns |
| --- | --- |
| `create_agent(handle, display_name, description="", *, avatar=None, starters=None)` | `AgentInfo` |
| `agents()` | `list[AgentInfo]` |
| `agent(agent_id)` | `AgentInfo` |
| `update_agent(agent_id, *, display_name=None, description=None, avatar=None, starters=None)` | `AgentInfo` |
| `delete_agent(agent_id)` | `None` |
| `connect(agent_id, *, webhook_url=None)` | the binding secret, **once** |
| `disconnect(agent_id)` | `None` |
| `create_code(agent_id, *, payload=None, max_uses=None, expires_in=None)` | `Code` |
| `codes(agent_id)` | raw dicts, not `Code` objects |
| `revoke_code(agent_id, token_id)` | `None` |
| `upload_avatar(path)` | a media id |

`AgentInfo` carries `id`, `handle`, `display_name`, `description`, `status`,
`has_avatar` and `starters`. `status` is `idle`, `connected`, `unreachable`, or
`None` when no backend has ever been attached.

`Code` carries `id`, `code`, `url`, `qr_png`, `max_uses` and `use_count`, plus
`png_bytes()` for writing the QR to a file.

```python
code = cuckoo.create_code(agent.id, payload={"customer_ref": "SBI-8812"}, max_uses=1)
open("card.png", "wb").write(code.png_bytes())
```

## Things that trip people up

- **`conv.stream()` is not awaited.** Use `async with`.
- **`agent.send` and `agent.upload` only work while the agent is running.**
  Calling them from a script outside `run()` raises. To send from elsewhere,
  use the HTTP API directly.
- **Webhook bindings do not work with this SDK.** `Agent` speaks socket mode
  only. Pointed at a webhook binding it is refused and exits quietly.
- **`file.save("/tmp")` writes into the directory**, under the file's own name.
  Pass a full path if you want to choose the name.
- **`codes()` returns dictionaries**, unlike `create_code`.
