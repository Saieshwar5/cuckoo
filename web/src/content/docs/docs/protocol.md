---
title: Protocol
description: The wire format an agent backend speaks to the hub. Endpoints, events, frames and errors.
---

This is the Cuckoo Agent Protocol as the hub implements it today. Everything
here is verified against the running server. Anything absent from this page
does not exist, whatever an older draft may say.

The protocol is versioned in the path. Only `/v1` exists.

## Authentication

Three credentials, all sent as `Authorization: Bearer <token>`.

| Prefix | Held by | Unlocks |
| --- | --- | --- |
| `ses_tok_` | A signed-in person, in the app | `/v1/client`, and `/v1/mgmt` |
| `mgt_tok_` | A server, as an API key | `/v1/mgmt` only |
| `bnd_sec_` | A running backend | `/v1/agent` only |

Each is 256 bits of randomness, shown once, and stored only as a hash.

Request bodies must be JSON. **Unknown fields are rejected**, so a typo in a
field name is an error rather than a silent no-op. Bodies are capped at 1 MiB.

## Agent API

Everything a backend can do, under `/v1/agent`.

| Method | Path | What |
| --- | --- | --- |
| GET | `/me` | Who this binding is |
| GET | `/events?since=evt_…` | Catch up over plain HTTP |
| GET | `/socket` | The WebSocket, and the normal way to work |
| GET | `/conversations/{id}` | One conversation and its participants |
| GET | `/conversations/{id}/messages` | History, newest first |
| POST | `/conversations/{id}/messages` | Send, or start a stream |
| POST | `/conversations/{id}/activity` | Say what the agent is doing: thinking, working, idle |
| POST | `/conversations/{id}/typing` | The first version of `/activity`: start or stop |
| POST | `/messages/{id}/append` | Add to a stream |
| POST | `/messages/{id}/finish` | End a stream |
| POST | `/media` | Upload a file |
| GET | `/media/{id}` | Download one |

There is no endpoint that lists conversations. Learn them from
`conversation.joined` and keep your own list.

### The socket

Connect to `wss://<hub>/v1/agent/socket` with the binding secret in an
`Authorization` header. There is no handshake to perform: events start arriving
as JSON text frames straight away, including anything you missed while away.

Two things can go wrong before the socket opens. A binding that does not exist
answers `403 no_binding`. A binding in webhook mode answers
`403 binding_not_socket`, which usually means somebody switched modes and
forgot.

The server pings every 30 seconds and expects an answer within 15. Frames are
capped at 64 KiB.

Only one socket wins per agent. When a second connects, the older one is closed
with code **4001**. A binding that is revoked or replaced closes with **4002**.
Neither is worth retrying.

### Acknowledging

Each event you receive carries an id. Acknowledge it once you have handled it.

```json
{ "ack": "evt_..." }
```

An event that is not acknowledged within 30 seconds becomes due again and is
sent again, so delivery is at least once and your handlers must tolerate seeing
the same event twice. The hub leases up to 100 events at a time.

Acknowledge after the work is done, not on receipt. That way a crash mid-reply
replays the message rather than losing it.

### Sending over the socket

One frame shape, with a `cid` you choose to match replies to requests.

```json
{ "op": "send",         "cid": "c1", "conversation_id": "cnv_…", "text": "…",
  "reply_to": "msg_…", "idempotency_key": "…",
  "buttons": [[ … ]], "quick_replies": [ … ] }
{ "op": "stream.start", "cid": "c2", "conversation_id": "cnv_…", "reply_to": "msg_…" }
{ "op": "stream.delta", "message_id": "msg_…", "text": "…" }
{ "op": "stream.end",   "cid": "c3", "message_id": "msg_…", "buttons": [[ … ]] }
{ "op": "activity",     "cid": "c4", "conversation_id": "cnv_…", "state": "working",
  "label": "Checking the weather" }
{ "op": "typing",       "cid": "c5", "conversation_id": "cnv_…", "state": "start" }
```

Replies come back as `{"reply_to_cid": "c1", "ok": true, "message": {…}}`, or
with `ok: false` and an error. A `stream.delta` gets no reply unless it fails.
A frame that is not valid JSON is dropped in silence.

**Attachments cannot be sent over the socket.** A message carrying files must
go through `POST /v1/agent/conversations/{id}/messages`.

### Catching up over HTTP

`GET /v1/agent/events?since=evt_…` returns events in order, oldest first, up to
100 at a time, whatever their delivery state. It is how a backend that lost its
place recovers, and how a webhook receiver fills a gap.

## Events

Every event has the same envelope.

```json
{ "id": "evt_…", "type": "message.created", "created_at": "…",
  "agent_id": "agt_…", "data": { … } }
```

There are exactly three types.

### `message.created`

```json
"data": {
  "conversation": { "id": "cnv_…", "kind": "dm" },
  "message": {
    "id": "msg_…",
    "sender": { "kind": "user", "id": "usr_…", "display_name": "Priya" },
    "body": {
      "text": "…",
      "attachments": [ { "media_id": "med_…", "kind": "image",
                         "mime_type": "image/jpeg", "byte_size": 234567,
                         "file_name": "photo.jpg", "width": 1080, "height": 1920,
                         "has_thumbnail": true, "duration_ms": 0, "waveform": [] } ],
      "buttons": [[ { "id": "raise", "label": "Raise complaint", "style": "primary" } ]],
      "quick_replies": [ { "label": "Not now" } ],
      "action": { "button_id": "raise", "source_message_id": "msg_…" },
      "selected_button_id": "raise"
    },
    "reply_to": { "id": "msg_…", "sender_kind": "agent", "text_preview": "…" },
    "status": "complete",
    "truncated": false,
    "created_at": "…",
    "signature": "…"
  },
  "participants": [ … ]
}
```

Fields inside `body` are omitted when empty rather than sent as null. `status`
is `streaming` or `complete`. An attachment carries no URL: fetch the bytes
from `GET /v1/agent/media/{id}`. `signature` is the hub's seal over the
message; see [Signatures](#signatures).

You never receive your own messages.

### `conversation.joined`

```json
"data": {
  "conversation": { "id": "cnv_…", "kind": "dm" },
  "participants": [ … ],
  "pair_token": { "id": "tok_…", "payload": { "customer_ref": "SBI-8812" } }
}
```

`pair_token` is present only when the join came from a code, and `payload` is
null unless the code carried one. Somebody who blocks your agent and adds it
again joins with **no** `pair_token` at all.

### `conversation.left`

```json
"data": { "conversation": { "id": "cnv_…", "kind": "dm" }, "reason": "user_blocked" }
```

`user_blocked` is the only reason the hub emits. Treat it as final and stop
sending.

### `stop.requested`

```json
"data": { "conversation": { "id": "cnv_…", "kind": "dm" }, "message_id": "msg_…" }
```

The person pressed stop. By the time this arrives the hub has already done its
part: the reply you were writing is finished where it stood, marked `stopped`
(and `truncated`), and your next append to it is refused with `409 stopped`.
`message_id` names that reply, or is null when you had not started writing.

What is left is the work behind the words — a model call, a search, a booking
not yet made. Cancel it. Anything already done stays done, and saying so is
yours: "Stopped. Nothing was booked." Both SDKs cancel the handler running for
that conversation for you, and treat the `StoppedError` its next write raises
as a finished event rather than one to retry.

Activity, delivery and read state are not agent-facing events. They exist, but
they go to people's devices only.

## Sending a message

```json
POST /v1/agent/conversations/{id}/messages
{
  "text": "…",
  "attachments": ["med_…"],
  "idempotency_key": "…",
  "reply_to": "msg_…",
  "stream": false,
  "buttons": [[ { "id": "…", "label": "…", "style": "…" } ]],
  "quick_replies": [ { "label": "…" } ]
}
```

`201` when the message is created, `200` when an `idempotency_key` matched an
earlier send. `attachments` is an array of media id strings.

### Buttons

A button has **an id or a url, never both**. An id button sends you a tap; a
url button opens something on the device and tells you nothing.

Buttons are laid out as rows: at most three rows, one to three buttons each.
Labels are 1 to 40 characters. Ids match `[A-Za-z0-9_.:-]{1,64}` and must be
unique within a message. Styles are `default`, `primary` or `danger`.

A url may be at most 2048 characters, and its scheme must be `https`, `http`,
`upi` or `tel`. There is no link preview and no confirmation sheet.

A tap arrives as a `message.created` from the person, whose `text` is the
button's label and whose `body.action` names the button and the message it was
on. The original message gains `selected_button_id`, so a button cannot be
taken twice.

Quick replies are up to six plain labels. Tapping one sends its words as
ordinary text, with no action attached.

Only agents may send buttons and quick replies.

### Streaming

Start with `"stream": true`, or the `stream.start` frame. A start that carries
text, buttons, quick replies or attachments is refused.

```
POST /conversations/{id}/messages  {"stream": true}   → 201, status "streaming"
POST /messages/{id}/append         {"text": "…"}      → 204
POST /messages/{id}/finish         {"buttons": [[…]]} → 200, status "complete"
```

Buttons and quick replies are attached at the end, not the start. Attachments
cannot be attached to a stream at all.

A stream that is idle for **30 seconds** is finished by the hub and marked
`truncated`, and the app tells the person the reply was cut short. A row still
open five minutes after it began is finished from whatever the buffer holds.
Text beyond 8000 characters is clamped and also marked truncated.

Appending to a stream that is finished or not yours is `409 not_streaming`.
Appending to one the person stopped is `409 stopped`: stop working, and do not
retry.

### Activity

`POST /conversations/{id}/activity` says what the agent is doing, and returns
`204`:

```json
{ "state": "thinking" }
{ "state": "working", "label": "Checking the weather" }
{ "state": "idle" }
```

The person sees it under the agent's name, in the chat and in the chat list:
*thinking…*, *Checking the weather…*. While they see it, the send button is a
stop button. It expires by itself after **10 seconds** unless said again, so a
backend that crashes mid-thought never leaves it spinning; say it again every
few seconds while the work goes on. Nothing is stored.

Only `working` takes a label: one line, at most 40 characters, no links —
`invalid_label` otherwise. It is a status line, not a message. A stream says
*writing…* by itself, so there is no need to send an activity while one is
open. `conv.working("…")` in both SDKs does all of this for you.

`POST /conversations/{id}/typing` with `{"state": "start"}` or `"stop"` is the
first version of the same thing: start is `thinking`, stop is `idle`.

## Media

Upload with `POST /v1/agent/media`, either as raw bytes with `?name=photo.jpg`
or as a multipart form with a field called `file`. The response carries the
media record, including the id you attach to a message.

There is no presigned upload and no completion step. Bytes go through the hub.

Voice notes take `?duration_ms=` and `?waveform=3,9,40`, both of which come
from the recording device: the hub does not decode audio. Pass `?kind=audio`
to mark a WebM, Ogg or MP4 container as a recording rather than a video, which
is the one claim that changes a file's kind.

Download with `GET /v1/agent/media/{id}`, or `?variant=thumb` for the small
copy of a picture. You may read files on messages in your own conversations and
files you uploaded yourself.

The kind is decided from the bytes, not from what the caller says.

| Kind | Maximum |
| --- | --- |
| image | 16 MiB |
| audio | 16 MiB |
| video | 64 MiB |
| file | 64 MiB |

At most ten files per message, no duplicates, and each file may be sent once.
A file nobody sends is swept away after a day.

## Webhook transport

If your binding is in webhook mode, the hub posts each event to your URL.

| Header | Value |
| --- | --- |
| `X-Cuckoo-Event` | the event type |
| `X-Cuckoo-Event-Id` | `evt_…` |
| `X-Cuckoo-Timestamp` | seconds |
| `X-Cuckoo-Signature` | `sha256=<hex>` |

The signature is `HMAC-SHA256` over `timestamp + "." + body`. **The key is the
SHA-256 of your binding secret, not the secret itself.** Compare in constant
time and reject a timestamp that is far from now.

Answer `2xx` within 10 seconds. Redirects are not followed and count as a
failure. The URL must be HTTPS, and private, loopback and link-local addresses
are refused.

There is no webhook receiver in the Python SDK. You write this one yourself.

## Errors

Every failure has the same envelope.

```json
{ "error": { "code": "not_participant",
             "message": "You are not in this conversation.",
             "field": "buttons", "request_id": "…" } }
```

Match on `code`, show `message` to a person, quote `request_id` when asking for
help. `field` appears when one field is at fault.

| Status | Meaning |
| --- | --- |
| 401 | The credential is missing or wrong |
| 403 | The credential is fine, the action is not allowed |
| 404 | No such thing, or it is no longer live |
| 409 | It conflicts with something that already happened |
| 422 | The request is malformed or breaks a rule |
| 429 | Too fast. `Retry-After` says how long to wait |
| 500 | The hub's fault. `request_id` identifies it |

There is no 400 and no 413. An oversized body is `422 body_too_large` and an
oversized file is `422 file_too_large`.

Codes you will meet most: `no_binding`, `binding_not_socket`, `not_participant`,
`blocked`, `invalid_text`, `invalid_buttons`, `too_many_attachments`,
`not_streaming`, `stream_not_found`, `idempotency_key_reused`, `rate_limited`.

## Delivery

Every event is a row in an outbox, so nothing depends on your backend being up
at the moment it happens.

A failed delivery is retried after 1 second, then 5 seconds, 30 seconds, 2
minutes, 10 minutes, and hourly after that. The hub gives up 24 hours after the
event was created.

A binding that has been failing for five unbroken minutes is marked
`unreachable`, and the app tells people the agent is away and that what they
send will be delivered when it is back.

Any success clears that: a webhook that answers 2xx, a socket that connects, a
heartbeat, or an acknowledgement.

## Devices

A person may be signed in on up to ten devices at once. The client API, which
the app speaks, lists them at `GET /v1/client/devices` with a name, when each
was last seen, and which one is asking; `DELETE /v1/client/devices/{id}` ends
one, and `DELETE /v1/client/devices` ends every one but the caller's. Past ten,
the device that has gone longest without being used is signed out to make room.

None of this reaches an agent: a backend is told a person's id and display
name, and never which device they are holding.

## Retention

The hub keeps a message and its attachments for a rolling window, 90 days by
default, then deletes them, oldest first. History paging stops at the window:
`GET /conversations/{id}/messages` pages back no further, and `next_before` is
null there.

Each person has a budget for media they uploaded that still belongs to a live
message, 100 MB by default; each agent has one, 1 GB by default. Over budget,
the oldest files go until the total is under it. The message text stays, and
the file answers `404 media_not_found` afterwards. Profile pictures are never
touched.

Delivery records, the outbox rows behind [Delivery](#delivery), are deleted a
week after they reach a final state. A sweeper runs inside the hub every six
hours.

The hub is a window and a courier, not the archive. The phone keeps its own
cache, and your backend received every message when it was sent. Keep your own
copy of anything you will need later.

The client API, which the app speaks, sees the same window. A history page
there carries `"trimmed": true` when the conversation began before the window,
and `GET /v1/client/me/storage` answers with `media.used_bytes` and
`media.budget_bytes`, and `messages.kept_days` and `messages.kept_since`, the
earliest moment the window still covers.

### Signatures

Every message the hub creates carries a `signature`: an opaque string the hub
computes over the message's immutable content with a key only the hub holds.
It appears on agent-facing message JSON, in events and in the agent history
endpoint, as `"signature": "<base64url>"`, and is absent on messages created
before signing existed. The client API does not carry it.

Its purpose is the window above. An agent's owner may keep messages beyond it
and, in a later version of the protocol, serve them back to the person through
the hub. The hub will verify the signature before relaying, so an archive can
omit a message but never alter one.

Store it with the message and return it unchanged. Do not try to verify it
yourself: the key is the hub's. It is unrelated to the webhook signature above.

See [Limits](/docs/limits/) for the rate limits and every numeric bound in one
table.
