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

When an agent's backend connects, drops, or stops answering, everyone who
has a chat with it gets `{"type":"agent.status","data":{"agent_id":"agt_...","status":"connected"}}`;
`status` is `connected`, `idle`, `unreachable`, or `none` once the binding is gone.

The socket is a hint and the database is the record. After a disconnect the
app asks for what it missed and then resumes:

```bash
curl -s -H "$H" "localhost:8080/v1/client/conversations/cnv_.../messages?after=msg_...&limit=50"
```

### Sending a photo or a file

A file is uploaded first and named by the message that carries it, so a slow
photo never holds a half-sent message open, and a failed upload leaves
nothing behind.

```bash
curl -s -H "$H" -X POST --data-binary @bill.jpg 'localhost:8080/v1/client/media?name=bill.jpg'
# {"media":{"id":"med_...","kind":"image","width":1600,"height":1067,"has_thumbnail":true,...}}

curl -s -H "$H" -X POST localhost:8080/v1/client/conversations/cnv_.../messages \
  -H 'Content-Type: application/json' \
  -d '{"text":"is this right?","attachments":["med_..."]}'
```

A voice note carries two things the bytes cannot tell anyone: how long it
runs, and its shape. Both are measured by whatever did the recording — a
phone and a browser both hand them out for nothing while the microphone is
open — and travel with the upload, so a waveform is drawn without decoding
a single frame of audio:

```bash
curl -s -H "$H" -X POST --data-binary @note.m4a \
  'localhost:8080/v1/client/media?name=note.m4a&kind=audio&duration_ms=8200&waveform=2,40,90,15'
```

They are what the sender said, not what the hub checked, which is safe
because of what they are for: a wrong number draws a wrong label under a
waveform, and decides nothing about who may read anything. `kind=audio` is
the one claim that changes how a file is treated, and it can only ever turn
a video into a recording: WebM, Ogg and MP4 hold either, and telling which
means decoding a stranger's media.

The hub decides what a file is from its bytes, never from its name or from
what the caller claimed. Limits are the protocol's, so an agent behaves the
same on any hub: 16 MB for a picture or a recording, 64 MB for a video or a
document, ten files in one message. A picture is measured and given a small
copy, so a bubble is the right shape before the bytes arrive:

```bash
curl -s -H "$H" localhost:8080/v1/client/media/med_...              # the file
curl -s -H "$H" 'localhost:8080/v1/client/media/med_...?variant=thumb'  # the small copy
```

A file is readable by whoever uploaded it and by everyone in the conversation
it was sent into. Nobody else, and an id nobody may read answers exactly like
one that does not exist. Uploads no message ever claimed are removed after a
day.

The agent protocol has the same two endpoints under `/v1/agent`, so a backend
downloads what it was sent and sends files back.

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
{ "ack": "evt_..." }
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

Add `@agent.on_join` and the agent speaks first to whoever adds it.

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

### Running agents from your own systems

Everything above is the app's job by default. A company with fifty bots,
or a website that mints a code per customer, needs its servers to do it
instead. That is an API key: created once in the app, pasted into a
deployment, and used as a bearer token on `/v1/mgmt`.

```bash
export CUCKOO_KEY=mgt_tok_...        # from the app: Agents → the key icon
export CUCKOO_HUB=http://localhost:8080

cuckoo agents create sbi-cards "SBI Cards"
cuckoo agents connect agt_...                     # prints the backend's secret
cuckoo codes create agt_... --once --payload '{"customer_ref":"SBI-8812"}'
```

or from code:

```python
from cuckoo import Management

cuckoo = Management(key=os.environ["CUCKOO_KEY"], hub="https://hub.example.com")
agent = cuckoo.create_agent("sbi-cards", "SBI Cards")
secret = cuckoo.connect(agent.id)                 # give this to your backend
code = cuckoo.create_code(agent.id, payload={"customer_ref": "SBI-8812"})
print(code.url)                                   # show this as a QR
```

A key does exactly what its owner can do in the management API, and
nothing else: it cannot read a conversation, cannot speak for an agent,
and cannot create another key. Keys are made and revoked in the app, and
revoking one stops everything using it at once.

### Giving an agent a face

A picture is uploaded like any other file and pointed at from a profile.
An agent's is published with it; a person's is theirs.

```bash
cuckoo agents create sbi-cards "SBI Cards" --avatar logo.png
cuckoo agents avatar agt_... newlogo.png
```

The picture goes to `POST /v1/mgmt/media` — the management API's own
upload — because a company's servers hold an API key, and a key does what
its owner can do there. It would be a strange rule that let a key create a
bank's agent but not give it a face.

**An agent's picture is the one public thing in Cuckoo:**

```bash
curl -s localhost:8080/a/agt_.../avatar > logo.jpg   # no credential
```

That is deliberate and narrow. The card a stranger opens from a QR code
renders in a browser before they have an account, and a company asking for
trust with a grey disc and the word "Unverified" is asking too much. Only
the small copy is served: nobody needs the original of a face, and a public
route handing out sixteen megabytes on request is a way to be knocked over.

A person's photo is not like that. It is fetched from `/v1/client/media`
behind their own session, and today only they can see it — an agent is told
a display name and nothing more.

### Handing an agent to other people

An agent starts private: only its owner has a chat with it. A pair token
opens it up. The owner mints one, and the response carries the link and a
QR code of it:

```bash
curl -s -H "$H" -X POST localhost:8080/v1/mgmt/agents/agt_.../pair-tokens \
  -H 'Content-Type: application/json' \
  -d '{"payload":{"customer_ref":"SBI-8812"},"max_uses":1,"expires_in":3600}'
# → {"token":{...},"code":"pair_...","url":"http://localhost:8080/p/pair_...","qr_png":"data:image/png;base64,..."}
```

Leave out `max_uses` for a poster anyone may scan; set it to one and put
your own reference in `payload` for a code minted per customer. Someone
scanning it sees the agent's card, with its owner's name and an Unverified
label, and taps Add:

```bash
curl -s -H "$H2" localhost:8080/v1/client/pair/pair_...          # the card
curl -s -H "$H2" -X POST localhost:8080/v1/client/pair/pair_.../accept   # → their chat with it
```

The backend hears `conversation.joined`, with the payload from the code,
before the person says a word. That is where the SDK's `@agent.on_join`
runs; `examples/buttons` greets people by the reference in their code.
Blocking the agent (`POST /v1/client/agents/agt_.../block`) closes the chat
in both directions and sends the backend `conversation.left`; scanning the
code again reopens it. Revoking a token stops new scans and changes nothing
for people who already have the agent. The link opens a small page on the
hub for anyone without the app; `CUCKOO_PUBLIC_URL` is its base.

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

| Command                               | Where the app runs                           |
| ------------------------------------- | -------------------------------------------- |
| `make play EMAIL=...`                 | your phone, through Expo Go                  |
| `make web EMAIL=...`                  | a tab in this laptop's browser, for building |
| `make play EMAIL=... TARGET=emulator` | the Android emulator                         |

The app follows the phone's light or dark setting, dark by default, in
white, grey and black, and works like the chat apps people already have:
open a chat, watch a reply stream in, tap the buttons an agent offers,
long-press to reply. Create an agent from the Agents tab, generate its
secret on the Connect screen, paste the snippet into a terminal, and the
screen says Connected the moment your code speaks. Share it from its
profile as a QR code; someone else scans it, or pastes the link, sees who
is behind it, taps Add, and your agent greets them.

The emulator is a one-time install with no Android Studio and no sudo:
`make emulator-install` downloads a Java runtime, the SDK tools and one
Android 15 image (about 5 GB) into your home directory and creates a
device. `make emulator` boots it; `make emulator-stop` closes it.

## Commands

| Command                 | What it does                                                |
| ----------------------- | ----------------------------------------------------------- |
| `make up` / `make down` | Start / stop the local Postgres and Redis                   |
| `make dev`              | Run the server with hot reload                              |
| `make test`             | Run every test, including the file-size guard               |
| `make test-short`       | Only tests that need no database                            |
| `make lint`             | Run the linter                                              |
| `make gen`              | Regenerate typed database code from SQL                     |
| `make check`            | Full verification — run this before every commit            |
| `make ci`               | Local CI: everything above plus migrate-from-empty and lint |
| `make watch`            | Rerun tests on every file change                            |
| `make hooks`            | Install the git hooks (once per clone)                      |
| `make size-top`         | Show the longest source files                               |
| `make psql`             | Open a shell on the development database                    |
| `make site`             | Serve the website at http://localhost:4321                  |
| `make site-build`       | Build the website into `web/dist`                           |

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
web/             the website: landing page, docs, legal (see web/README.md)
deploy/          Caddy in front of the hub and the site (see deploy/README.md)
sdk/python/      the Python SDK: connect, receive, reply
protocol/        the published agent protocol spec (not started)
examples/echo/   the reference agent, and the protocol's smoke test
examples/stream/ an agent that answers a word at a time
examples/buttons/ an agent that asks before it acts
examples/welcome/ the agent a new account starts with
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
