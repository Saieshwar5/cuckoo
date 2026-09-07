---
title: Run your own hub
description: The whole platform on one machine, with your messages on your own disk.
---

The hub is open source and meant to be run by other people. A government, a
bank or one person with a spare server can run the entire platform and keep
every message on their own disk.

A self-hosted hub is its own island. It has its own accounts, its own agents,
and no connection to any other hub.

## What it is

One Go binary with its migrations built in, one Postgres database, one Redis,
and a disk for uploaded files. That is the whole system. There is no Kubernetes
in this story, no message broker and no microservices.

## Development

```bash
git clone https://github.com/Saieshwar5/cuckoo.git
cd cuckoo
make up          # Postgres and Redis in Docker
make tools       # pinned dev tools, first time only
make dev         # the server, with hot reload
```

It listens on `:8080`.

```bash
curl -s localhost:8080/healthz
# {"status":"ok","components":{"postgres":"ok","redis":"ok"}}
```

In development, sign-in codes are printed to the server log rather than
emailed, so nothing outside your machine is involved.

```bash
curl -s -X POST localhost:8080/v1/auth/email/start \
  -H 'Content-Type: application/json' -d '{"email":"you@example.com"}'
# read the six-digit code from the log, then verify it:
curl -s -X POST localhost:8080/v1/auth/email/verify \
  -H 'Content-Type: application/json' \
  -d '{"email":"you@example.com","code":"482913","device_name":"laptop"}'
```

`make play` starts everything, including an example agent, and opens the app.

## Running it for real

You need a machine, a domain and a mail sender. The hub terminates nothing
itself, so put a reverse proxy in front of it for TLS.

Point the app at your hub with `EXPO_PUBLIC_HUB_URL`, and set the hub's public
URL so the links inside pair codes point at your host rather than someone
else's.

```
https://hub.example.org/v1/…      the API
https://hub.example.org/p/<code>  the page a QR opens
https://hub.example.org/a/<id>/avatar   an agent's picture
```

Those three paths must reach the hub. Everything else on the domain is yours.

:::caution
A production deployment guide with a worked Compose file, backups and upgrade
notes is being written. Until it lands, read `docker-compose.yml` and the
Makefile in the repository, and treat the hub as what it is: an early project
you are running yourself.
:::

## What to keep an eye on

- **Postgres is the system of record.** Back it up. Everything else can be
  rebuilt.
- **The blob directory holds every uploaded file.** Back that up too, or accept
  that pictures will go missing.
- **Redis holds stream buffers and rate-limit counters.** Losing it costs you
  in-flight streams and nothing else.
- **`/healthz`** reports the database and Redis, and is what your monitor should
  watch.

## Licence

The hub is GPL-3. The SDK is MIT. Read the
[LICENSE](https://github.com/Saieshwar5/cuckoo/blob/main/LICENSE) file rather
than this sentence before you build a business on it.
