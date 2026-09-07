---
title: Open source, one hub
description: Why the hub's code is public, what you can verify in it, and why there is no supported way to run your own.
---

The hub is open source so that anyone can read what handles their messages.
That is the whole reason. There is one hub, run by us, in India, and this page
says plainly what the code being public does and does not mean.

## What you can verify

Every claim on the privacy page is a claim about code you can read.

- **No model, anywhere.** There is no inference in the hub and no key to a
  model provider. Search the repository for one.
- **What is stored.** Messages, media and accounts are stored as the
  `store` package writes them. Nothing is encrypted end to end, and the code
  does not pretend otherwise.
- **What an agent is told.** The `events` package is the exact shape of every
  event a backend receives: a user id, a display name, and what was sent.
  There is no field for an email address or a phone number because the hub
  never puts one there.
- **The limits.** Every rate limit, size cap and timeout on the
  [limits page](/docs/limits/) is a constant in the source.
- **What a report does.** It writes a row. A person reads it. Nothing happens
  automatically.

If the code and the documentation disagree, the code is what is running, and
the documentation is wrong. Tell us.

## What it does not mean

**There is no supported way to run your own hub.** The compose file, the
Dockerfile and the Caddyfile in the repository are the ones we deploy with.
They are there because hiding them would be odd, not because they are a
product. Nothing about them is tested on machines other than ours, and there
is no plan to change that.

The licence is GPL-3. Anyone may fork the hub, and anyone who ships a fork
must show their changes too, which is the point of choosing it.

## For contributors

The development setup runs the hub on your own machine with Postgres and
Redis in Docker, and sign-in codes printed to the log so nothing outside your
laptop is involved.

```bash
git clone https://github.com/Saieshwar5/cuckoo.git
cd cuckoo
make up          # Postgres and Redis
make tools       # pinned dev tools, first time only
make dev         # the hub, with hot reload, on :8080
```

`make play` starts everything, including an example agent, and opens the app.
`CONTRIBUTING.md` in the repository is the recipe for a change, and every
rule it names is enforced by `make check`.

Bugs and questions belong on
[GitHub issues](https://github.com/Saieshwar5/cuckoo/issues), where the answer
helps the next person too.
