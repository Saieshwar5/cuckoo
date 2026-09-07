---
title: Concepts
description: Agent, binding, code, conversation. Seven words that explain the whole system.
---

Seven words carry the whole system. Learn them once and the rest of the
documentation reads quickly.

## Agent

The thing a person sees in their chat list: a handle, a display name, a
description, a picture, and up to four starters. It is created by a person in
the app or by a server through the management API, and it belongs to the
account that created it.

An agent has no intelligence of its own. It is an identity and a mailbox.

A handle is permanent. Everything else can be edited later.

## Binding

The link between an agent and the code that answers for it. Creating a binding
returns a **binding secret**, shown once, and that secret is what your process
authenticates with.

There are two modes.

- **Socket.** Your program dials out to the hub over a WebSocket and stays
  connected. It works from a laptop, a container, or a server behind any
  firewall. The Python SDK speaks this mode.
- **Webhook.** The hub posts each event to an HTTPS URL you host, signed with
  a header your code checks. You write the receiver yourself.

Connecting again replaces the binding and kills the old secret. That is how you
rotate a credential or change vendors. The agent, its handle and its history
stay exactly where they are.

## Conversation

One private chat between one person and one agent. It exists because the person
added the agent. Your backend sees a conversation id on every event, and that
id is the person as far as your code is concerned.

Today every conversation is a direct message. Groups are being built.

## Window

What the hub keeps of a conversation. A message and its files stay on the hub
for 90 days and are then deleted, oldest first. The phone keeps its own copy of
recent chats, and the agent's owner received every message when it was sent
and keeps their own. The hub is a window and a courier, not the archive: if
your backend needs what was said, store it.

## Code

What puts an agent in someone's chat list. A code is a link, and a QR image of
that link, that you mint from the management API.

- A **poster code** has no limit and no expiry. Print it. Anyone can scan it.
- A **personalised code** carries a payload you choose and is usually good for
  one use. Mint one per logged-in customer, and your backend is told who they
  are the moment they add the agent.

Codes can be revoked. Revoking one stops new people adding the agent and leaves
existing conversations untouched.

## Event

What your backend receives. There are exactly three kinds.

| Event | Meaning |
| --- | --- |
| `message.created` | Somebody sent a message in a conversation you are in. |
| `conversation.joined` | Somebody added your agent. A new chat exists. |
| `conversation.left` | Somebody blocked your agent. Stop sending. |

Every event has an id. Over a socket you acknowledge each one, and anything you
do not acknowledge is delivered again. That is how a backend that restarts
mid-message loses nothing.

## API key

A management credential belonging to a person's account, created in the app.
It is shown once, it can be revoked, and it lets a server create and manage
agents without a phone in the loop.

An API key cannot mint another API key. That is deliberate.

## The three secrets, side by side

| Prefix | What it is | Where it is used |
| --- | --- | --- |
| `ses_tok_` | A signed-in person's session | The app |
| `mgt_tok_` | An API key | Creating and managing agents |
| `bnd_sec_` | A binding secret | The running backend |

Each one is shown once and stored only as a hash. If you lose one, make a new
one; nobody can recover it for you.
