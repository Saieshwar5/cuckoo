# Cuckoo

A chat platform built for AI agents.

Messaging apps were designed for people, and bots are bolted on afterwards:
they cannot start a conversation, cannot stream a reply as it is written,
cannot ask a structured question, and cannot be full members of a group.
Cuckoo inverts that. Agents are first-class, and the protocol they speak is the
product — any company, government or individual can connect their own agent
backend to it.

Cuckoo moves messages. It never runs inference: the intelligence lives in
backends run by whoever owns the agent.

**Status:** early. The server foundation is in place — configuration, database
layer, HTTP stack, authentication seam, and a fully tested example endpoint.
Agents, conversations and messages come next.

## Quick start

Requires Go 1.26+ and Docker.

```bash
make up          # start Postgres and Redis
make tools       # install pinned dev tools into ./bin (first time only)
make dev         # run the server with hot reload
```

The server listens on `:8080`. Check it is alive:

```bash
curl -s localhost:8080/healthz
# {"status":"ok","components":{"postgres":"ok","redis":"ok"}}
```

Sign in with an email code. In development the code is printed to the
server log (`CUCKOO_MAIL=console`) instead of being sent, so nothing outside
this machine is involved:

```bash
curl -s -X POST localhost:8080/v1/auth/email/start -H 'Content-Type: application/json' \
  -d '{"email":"priya@example.com"}'
# read the six-digit code off the server log, then:
curl -s -X POST localhost:8080/v1/auth/email/verify -H 'Content-Type: application/json' \
  -d '{"email":"priya@example.com","code":"482913","device_name":"laptop"}'
# → {"token":"ses_tok_...","user":{...},"is_new":true}
export H="Authorization: Bearer ses_tok_..."
curl -s -H "$H" localhost:8080/v1/client/me
```

A new address becomes a new account. Signing in on another device signs the
first one out. `POST /v1/auth/logout` ends the session.

In development the server also accepts an `X-Dev-User` header naming any user,
so scripts can skip signing in:

```bash
export H="X-Dev-User: usr_..."
curl -s -H "$H" localhost:8080/v1/client/me

# Create an agent; this also opens the DM between you and it.
curl -s -H "$H" -X POST localhost:8080/v1/mgmt/agents \
  -H 'Content-Type: application/json' \
  -d '{"handle":"helper","display_name":"Helper"}'

# The chat list, most recently active first.
curl -s -H "$H" localhost:8080/v1/client/conversations

# Send a message and page the history back, newest first.
curl -s -H "$H" -X POST localhost:8080/v1/client/conversations/cnv_.../messages \
  -H 'Content-Type: application/json' -d '{"text":"hello"}'
curl -s -H "$H" "localhost:8080/v1/client/conversations/cnv_.../messages?limit=50"
```

That header exists only when `CUCKOO_ENV=dev`; in any other environment only
session tokens are accepted.
### Live updates for the app

The app holds one WebSocket at `/v1/client/socket`, authenticated like any
other request, and receives a frame the moment anything happens in one of its
conversations. It sends nothing back; everything the app does goes through
the REST API.

```json
{"type":"ready","data":{"user_id":"usr_..."}}
{"type":"message.created","data":{"conversation_id":"cnv_...","message":{...}}}
{"type":"delivery.updated","data":{"conversation_id":"cnv_...","message_id":"msg_...","delivery_status":"delivered"}}
```

The socket is a hint and the database is the record. After a disconnect the
app asks for what it missed and then resumes:

```bash
curl -s -H "$H" "localhost:8080/v1/client/conversations/cnv_.../messages?after=msg_...&limit=50"
```

### Receiving events as an agent backend

Connect a webhook backend to an agent and the hub POSTs every message in the
agent's conversations to it. The secret is shown once.

```bash
curl -s -H "$H" -X POST localhost:8080/v1/mgmt/agents/agt_.../binding \
  -H 'Content-Type: application/json' \
  -d '{"mode":"webhook","webhook_url":"http://localhost:9000/cuckoo"}'
```

Each request carries `X-Cuckoo-Event`, `X-Cuckoo-Event-Id`, `X-Cuckoo-Timestamp`
and `X-Cuckoo-Signature: sha256=<hex HMAC-SHA256>` over
`<timestamp>.<raw body>`. The signing key is the SHA-256 of the binding
secret, so the plaintext secret is never stored on the hub:

```python
key = hashlib.sha256(secret.encode()).digest()
expected = hmac.new(key, f"{ts}.{body}".encode(), hashlib.sha256).hexdigest()
```

Return any 2xx within ten seconds to acknowledge. Anything else is retried
for a day: 1s, 5s, 30s, 2m, 10m, 1h, then hourly. Five minutes of failures
marks the binding unreachable until the next success. A backend that was
away reads what it missed, oldest first:

```bash
curl -s -H "Authorization: Bearer bnd_sec_..." \
  "localhost:8080/v1/agent/events?since=evt_...&limit=100"
```

In production the hub refuses to post to private, loopback and link-local
addresses. `http://localhost` works only with `CUCKOO_ENV=dev`.

A backend that cannot receive a webhook, such as a laptop behind a router,
connects outward instead. Set a socket binding and open a WebSocket at
`/v1/agent/socket` with the same bearer header. Everything pending is pushed
first, in order; then events arrive as they happen. Acknowledge each one, or
it is pushed again after thirty seconds:

```json
{"ack": "evt_..."}
```

The Python SDK does all of this, and `examples/echo` is the whole of an agent:

```python
from cuckoo import Agent

agent = Agent(secret="bnd_sec_...", hub="http://localhost:8080")

@agent.on_message
async def handle(msg, conv):
    await conv.send(f"You said: {msg.text}")

agent.run()
```

To reply, post into the conversation the event named. Send an
`idempotency_key` and a retry after a lost response returns the same message
(200) instead of creating another (201). History starts from when the agent
joined.

```bash
curl -s -X POST localhost:8080/v1/agent/conversations/cnv_.../messages \
  -H "Authorization: Bearer bnd_sec_..." -H 'Content-Type: application/json' \
  -d '{"text":"I can see the deduction. It will auto-reverse in 3 working days.",
       "idempotency_key":"reply-8812-1"}'
curl -s -H "Authorization: Bearer bnd_sec_..." \
  "localhost:8080/v1/agent/conversations/cnv_.../messages?limit=50"
```

A reply from a model arrives over seconds. Stream it, and the person watches
the bubble fill instead of waiting for it. Over HTTP: start with
`{"stream": true}`, `POST /v1/agent/messages/msg_.../append` each piece, then
`POST /v1/agent/messages/msg_.../finish`. Over the socket the same three are
`stream.start`, `stream.delta` and `stream.end` frames, which is what the SDK
uses:

```python
async with conv.stream() as reply:
    async for token in model.generate(msg.text):
        await reply.append(token)
```

The app sees `message.started`, `message.delta` and `message.completed`
frames. A stream that goes quiet for thirty seconds is finished by the hub
with what it has and marked `truncated`, so a bubble never spins forever.
`examples/stream` shows it word by word.

An agent asks with buttons, and the answer comes back as an id it can
match, never as text it has to interpret. A person's tap is an ordinary
message whose text is the label and whose body names the choice; the
question records which button was taken. Quick replies are suggested
answers sent as plain text. Any message may quote another with `reply_to`,
and readers get a preview of the original. An agent may show a typing
indicator, which expires on its own after ten seconds.

```python
await conv.typing()
await conv.send("Which account?", reply_to=msg.id,
                buttons=[[("acc-salary", "Salary account", "primary"), ("acc-savings", "Savings")]],
                quick_replies=["Neither"])
# later, the tap arrives as a message:
if msg.action:
    await conv.send(f"Checking {msg.action.button_id}", reply_to=msg.id)
```

`examples/buttons` is the whole exchange.

Senders are rate limited per identity: a person may burst 30 messages and
then send one every two seconds; an agent may burst 120 and then 60 a second.
Over the limit is a 429 with a `Retry-After` header. Starting with any other value
fails immediately rather than falling back to it.

Postgres and Redis are published on **5433** and **6380**, off the default
ports, so Cuckoo can run alongside another project's containers.

### The app

One command, and a phone with Expo Go on the same wifi:

```bash
make play EMAIL=you@example.com
```

That starts Postgres and Redis, builds and starts the hub, signs you in
(reading the code from the hub's own log), creates an Echo agent and runs
it, and starts the app's dev server with your laptop's wifi address filled
in. Scan the QR code with Expo Go and sign in with the same email; the code
is printed in green in that terminal. Then, from another terminal:

```bash
make say TEXT="hello from the laptop"
```

and watch the chat list on the phone update as the agent answers. Ctrl+C
stops everything; `make stop` cleans up if something was left behind.

The same command runs the app elsewhere:

| Command | Where the app runs |
|---|---|
| `make play EMAIL=...` | your phone, through Expo Go |
| `make web EMAIL=...` | a tab in this laptop's browser, for building |
| `make play EMAIL=... TARGET=emulator` | the Android emulator |

The emulator is a one-time install with no Android Studio and no sudo:
`make emulator-install` downloads a Java runtime, the SDK tools and one
Android 15 image (about 5 GB) into your home directory and creates a
device. `make emulator` boots it; `make emulator-stop` closes it.

## Commands

| Command | What it does |
|---|---|
| `make up` / `make down` | Start / stop the local Postgres and Redis |
| `make dev` | Run the server with hot reload |
| `make test` | Run every test, including the file-size guard |
| `make test-short` | Only tests that need no database |
| `make lint` | Run the linter |
| `make gen` | Regenerate typed database code from SQL |
| `make check` | Full verification — run this before every commit |
| `make ci` | Local CI: everything above plus migrate-from-empty and lint |
| `make watch` | Rerun tests on every file change |
| `make hooks` | Install the git hooks (once per clone) |
| `make size-top` | Show the longest source files |
| `make psql` | Open a shell on the development database |

## Layout

```
server/          the hub: one Go binary, migrations embedded
  cmd/cuckoo/    entry point
  internal/
    api/         HTTP: routing, middleware, request and response handling
    auth/        credentials to caller
    signin/      email codes and sessions
    mail/        sending the few emails the hub sends
    config/      environment to typed settings
    conversations/ conversations, participants and messages
    delivery/    the outbox worker: webhook delivery, retries, catch-up
    domain/      shared error model and identifiers
    events/      what a backend receives: the event envelope and payloads
    principal/   who is calling
    ratelimit/   token buckets in Redis
    realtime/    live updates: the event bus and the connection hub
    store/       database access, migrations, generated queries
    testutil/    test harness: transactional stores, HTTP client, fixtures
    agents/      agent identities and the bindings that connect them to backends
    users/       the first business package, and the pattern for the rest
apps/mobile/     the app: React Native with Expo (see apps/mobile/README.md)
sdk/python/      the Python SDK: connect, receive, reply
protocol/        the published agent protocol spec (not started)
examples/echo/   the reference agent, and the protocol's smoke test
examples/stream/ an agent that answers a word at a time
examples/buttons/ an agent that asks before it acts
scripts/         developer tooling
```

## How it is built

- **One hub, self-hostable.** Not federated. The server is a single binary with
  its migrations embedded, so running a private hub is one file and a compose
  file.
- **The database is real in tests.** The hard bugs in a messaging system live in
  ordering, constraints and delivery state, and a mock agrees with whatever the
  code believes. Every test runs inside a transaction that is rolled back, so
  they are isolated, leave nothing behind, and stay fast.
- **No file over 500 lines.** Enforced by `scripts/check-file-size.sh`, which
  runs as part of `make test`. A long file means a package has taken on a
  second job.
- **Nothing speculative.** Each migration adds only what the code in the same
  change uses. No placeholder packages, no unused containers.
- **Verification runs locally.** There is no hosted CI. `make hooks` installs a
  fast pre-commit check and a full pre-push one; `make ci` is the complete run,
  including migrating a fresh database from empty.

See [CONTRIBUTING.md](CONTRIBUTING.md) for how to add a feature.

## Licence

GPL-3.0. See [LICENSE](LICENSE).
