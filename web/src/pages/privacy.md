---
layout: ../layouts/Prose.astro
title: Privacy
description: What Cuckoo stores, who can see it, and how to get rid of it.
updated: 7 September 2026
---

This describes what the hosted Cuckoo hub does with your information. It is
written to be read, not to be survived.

Somebody running their own hub answers for their own copy. This page covers
ours.

## What we hold

**Your account.** An email address, a display name, a picture if you set one,
and the language your phone is set to. That is the account. There is no
password and no phone number.

**Your messages.** Text, pictures, files and voice notes, kept so they are
there when you open the app. They are stored in the clear on the server.
Cuckoo is a reliable relay, not a blind one. Anyone with access to the machine
could read them, and we do not pretend otherwise.

**Your chat list.** Which agents you have added, which you muted, pinned,
archived, blocked or reported.

**How the app reached us.** A device name you chose, and ordinary server logs
with an address and a timestamp.

We do not use trackers, advertising identifiers or third-party analytics. There
is nothing on this website that watches you either.

## What an agent learns about you

This is the part worth reading twice.

An agent is somebody else's program. When you talk to one, it receives your
**display name**, an id that identifies you to that agent, and **what you send
it**: your words, your pictures, your voice notes.

It does not receive your email address, your phone number, your other
conversations, or anything you have not sent it.

If the agent was added through a personalised link, whoever made that link may
already know who you are on their own system. That is how a bank can greet you
by name.

What the agent's owner does with what you send is governed by their rules, not
ours. An agent shows its owner's name and, until verification exists, the label
**Unverified**. Treat it the way you would treat any account you do not
recognise.

## What we do with it

We move your messages, keep them so you can read them again, and keep the
service working. That is all. We do not sell anything, we do not train models
on your messages, and we do not read them except when we must investigate a
report or a fault.

## Reports

When you report an agent, we receive that report and the conversation it
concerns, so somebody can look at it.

## How long we keep things

Messages stay until you delete them or delete your account. A file nobody sends
is removed within a day. Logs are kept for a short operating window.

Deleting a message deletes it for you. The agent already received it, and we
cannot reach into somebody else's server. The app says so at the time.

## Deleting everything

Settings, then Account, then Delete account, and it is gone: your account, your
sessions and your agents. See [the deletion page](/delete-account/).

## Children

Cuckoo is not for children under 13.

## Where this runs

The hosted hub runs on servers in India.

## Changes

If this changes in a way that matters, the date at the top changes and the app
will say so.

## Asking us something

Write to the address on the [support page](/support/) and a person will answer.

---

*This is an early draft written alongside the software, and has not been
reviewed by a lawyer. It describes what the system actually does today. If you
are relying on it for anything consequential, ask us and we will tell you
plainly.*
