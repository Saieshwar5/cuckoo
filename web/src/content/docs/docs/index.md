---
title: Overview
description: What Cuckoo is, what it is not, and where to start depending on what you are building.
---

Cuckoo is a chat app where AI agents are first-class, and an open protocol for
connecting them. The hub moves messages between people and agent backends. It
never runs a model: the intelligence, and its bill, belong to whoever owns the
agent.

That means three things for you as a builder.

- **You write an ordinary program.** It connects out to the hub, receives
  messages, and replies. There is no inbound port to open and no queue to run.
- **You keep your own state.** The hub tells you a conversation id and a user
  id. Everything else about that person is yours to store.
- **You can read every line of it.** The hub is open source, so what it does
  with a message is something you can check rather than trust.

## Where to start

| If you want to… | Read |
| --- | --- |
| See something working in ten minutes | [Quickstart](/docs/quickstart/) |
| Understand the pieces before you type | [Concepts](/docs/concepts/) |
| Attach your own harness to an agent in the app | [Your own agent](/docs/your-own-agent/) |
| Put an agent in front of customers | [For companies](/docs/for-companies/) |
| Know who is talking to your backend | [Identity](/docs/identity/) |
| Read the wire format | [Protocol](/docs/protocol/) |
| See what the hub does with your data | [Open source, one hub](/docs/open-source/) |

## What Cuckoo does not do

Being clear about this early saves you an afternoon.

- **No inference.** There is no model in the hub, no prompt, no key to add.
- **No end-to-end encryption.** Traffic is over TLS and messages are readable
  on the hub. It is a reliable relay, not a blind one.
- **No identity beyond a name.** Your backend learns a user id and a display
  name. Not an email, not a phone number, not their other chats.
- **No agent-to-agent messaging yet.** Agents talk to people. Teams of agents
  that talk to each other are planned, not built.
- **No group chats or push notifications yet.** Both are being built.

## Status

Cuckoo is early. The protocol is versioned at `/v1` and the parts documented
here are built and tested, but they can still change while the version stays
at zero. Anything not on this site does not exist yet, whatever a stray
comment in the source may suggest.
