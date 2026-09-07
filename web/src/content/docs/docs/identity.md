---
title: Identity
description: What your backend learns about the person talking, and the three ways to link it to your own customer record.
---

There are two identities in play. Cuckoo knows a person as a user id. You know
them as a customer number, an order, an account. The hub never learns the
second one. Linking them once is your job, and after that every message is
recognised.

## What you actually receive

Every message carries four things about its sender and no more.

| Field | Example | Meaning |
| --- | --- | --- |
| `conv.id` | `cnv_…` | Which chat this is |
| `msg.sender.kind` | `user` | A person, not another agent |
| `msg.sender.id` | `usr_…` | This person's permanent id on the hub |
| `msg.sender.display_name` | `Priya` | The name they chose in the app |

That is the whole of it. No email address, no phone number, no other
conversations, no profile. The id is issued by the hub, cannot be forged by the
person, and is the same every time they talk to your agent.

Store your link by `sender.id` rather than by conversation id. In a direct
message the two are equivalent, but the user id keeps working when group chats
arrive.

## Way 1: a personalised code

The best way, and the reason codes carry a payload. Use it whenever the person
is already logged in somewhere you control.

A customer taps "Chat on Cuckoo" on your site. Your server mints a code for
that moment, with your own reference inside it.

```python
code = cuckoo.create_code(
    agent.id,
    payload={"customer_ref": "SBI-8812"},
    max_uses=1,
    expires_in=600,
)
return redirect(code.url)
```

They add the agent, and your backend is told who they are before they say a
word.

```python
@agent.on_join
async def joined(conv, token):
    ref = (token.payload or {}).get("customer_ref") if token else None
    if ref:
        await link(conv, ref)
        await conv.send("Hi Priya. Your card ending 8812 is active.")
    else:
        await conv.send("Hi. What is your registered mobile number?")
```

Because the code was minted inside a logged-in session, holding it is evidence
the person was logged in. Keep `max_uses=1` and a short expiry so a forwarded
link is worth little, and confirm on your own side before anything that moves
money.

## Way 2: a poster code, then ask

A code on a poster, a printed statement or a public page can be scanned by
anyone, so the join arrives with no payload. Ask once, in the chat, and verify
through a channel you already own.

```python
@agent.on_message
async def handle(msg, conv):
    ref = await lookup(msg.sender.id)
    if ref is None:
        await start_or_check_otp(msg, conv)   # your own SMS or email
        return
    await conv.send(answer(ref, msg.text))
```

Send the one-time code over your own SMS or email, never over the chat. The
chat is where they type it back. Once it checks out, store the link against
`msg.sender.id` and never ask again.

## Way 3: do not link at all

Many agents have no idea who you are and do not need one. A menu, a timetable,
a weather agent, a public information line. The display name is enough to be
polite, and the conversation id is enough to remember what was said earlier.

Not asking is a feature. It is also the fastest thing to ship.

## Rules worth knowing before you design this

- **A join event is not guaranteed to be personalised.** If somebody blocks
  your agent and later adds it again, `conversation.joined` arrives with no
  pair token at all. Handle a missing payload every time.
- **A personalised code is spent only by a genuinely new contact.** Somebody
  who removed your agent and re-added it does not consume a use, and does not
  produce a fresh join event.
- **Display names are not unique and not verified.** Two people can both be
  Priya. Never use a display name as a key.
- **Blocking is final until they undo it.** After `conversation.left` your
  sends are refused. Drop the conversation from your queues.
- **The person can delete their account.** Treat a user id that stops
  responding as gone, and keep your own record only as long as you need it.

## A worked link table

If you want one shape to copy, this is it.

| Column | Value |
| --- | --- |
| `cuckoo_user_id` | `usr_…`, primary key |
| `customer_ref` | your own id |
| `linked_via` | `code` or `otp` |
| `linked_at` | timestamp |
| `conversation_id` | `cnv_…`, handy for logs |

One row per person, written once, read on every message.
