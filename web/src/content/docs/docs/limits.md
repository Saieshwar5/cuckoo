---
title: Limits
description: Every rate limit, size cap and timeout in one table.
---

Every number the hub enforces, in one place. They are what the running server
does today, not a policy statement.

## Rate limits

Each is a token bucket: a burst you can spend at once, refilling at a steady
rate. Exceeding one gives `429` with a `Retry-After` header.

| What | Rate | Burst | Counted per |
| --- | --- | --- | --- |
| Agent sends, and starting a stream | 60/s | 120 | agent |
| Stream appends | 200/s | 400 | agent |
| Typing | 200/s | 400 | agent |
| Agent uploads | 5/s | 40 | agent |
| A person's sends | 0.5/s | 30 | user |
| A person's uploads | 0.5/s | 20 | user |
| Sign-in codes | 1 per 2 min | 5 | email address |

These are per agent, not per conversation. An agent messaging a hundred
thousand people needs to pace itself.

## Text and controls

| Thing | Limit |
| --- | --- |
| Message text | 8000 characters |
| Button rows | 3 |
| Buttons per row | 1 to 3 |
| Quick replies | 6 |
| Button or quick-reply label | 1 to 40 characters |
| Button id | 1 to 64 characters, `[A-Za-z0-9_.:-]`, unique per message |
| Button url | 2048 characters, scheme `https`, `http`, `upi` or `tel` |
| Reply preview | Cut at 100 characters |
| Idempotency key | 200 characters |
| Page size | 50 by default, 100 at most |

## Files

| Kind | Maximum |
| --- | --- |
| image | 16 MiB |
| audio | 16 MiB |
| video | 64 MiB |
| file | 64 MiB |

| Thing | Limit |
| --- | --- |
| Attachments per message | 10, no duplicates |
| File name | 255 characters |
| Recording length | 10 minutes |
| Waveform | 64 values, each 0 to 100 |
| Image size | 50 megapixels |
| Thumbnail | 480 px on the long side |
| Unsent upload kept for | 1 day |

## Agent fields

| Field | Rule |
| --- | --- |
| handle | 3 to 32 characters, `[a-z0-9][a-z0-9_-]*`, permanent, some words reserved |
| display name | 1 to 80 characters |
| description | up to 500 characters |
| starters | up to 4, each 1 to 40 characters, no duplicates |

## Codes

| Field | Rule |
| --- | --- |
| payload | any JSON under 4096 bytes |
| max uses | 1 to 1,000,000, or unlimited if left out |
| expiry | any time in the future, or never if left out |
| QR image | 512 px |

## Timeouts

| Thing | Time |
| --- | --- |
| Stream idle before the hub finishes it | 30 s |
| Stream open before the hub force-finishes it | 5 min |
| Typing indicator | 10 s |
| Socket ping interval | 30 s |
| Socket ping timeout | 15 s |
| Unacknowledged event redelivered after | 30 s |
| Webhook response deadline | 10 s |
| Sign-in code validity | 10 min, 5 guesses |
| Session | 90 days |

## Delivery

| Attempt | Delay before the next try |
| --- | --- |
| 1 | 1 s |
| 2 | 5 s |
| 3 | 30 s |
| 4 | 2 min |
| 5 | 10 min |
| 6 and after | 1 h |

The hub stops trying 24 hours after the event was created. A binding failing
for five unbroken minutes is marked `unreachable`.

## Protocol limits worth designing around

- Request bodies are capped at 1 MiB, socket frames at 64 KiB.
- Unknown JSON fields are rejected outright.
- One socket per agent. A second connection closes the first.
- Delivery is at least once, so handlers must be safe to run twice.
