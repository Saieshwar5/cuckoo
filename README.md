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

Real authentication is not built yet. In development the server accepts an
`X-Dev-User` header naming the caller, so the API can be exercised with `curl`
long before there is an inbox or a login screen:

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

This bypass exists only when `CUCKOO_ENV=dev`. Starting with any other value
fails immediately rather than falling back to it.

Postgres and Redis are published on **5433** and **6380**, off the default
ports, so Cuckoo can run alongside another project's containers.

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
    config/      environment to typed settings
    conversations/ conversations, participants and messages
    domain/      shared error model and identifiers
    principal/   who is calling
    store/       database access, migrations, generated queries
    testutil/    test harness: transactional stores, HTTP client, fixtures
    agents/      agent identities and the bindings that connect them to backends
    users/       the first business package, and the pattern for the rest
apps/mobile/     React Native app (not started)
sdk/             agent SDKs, Python first (not started)
protocol/        the published agent protocol spec (not started)
examples/        reference agents (not started)
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
