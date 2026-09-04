# Contributing

## Adding a feature

Almost everything in Cuckoo is the same shape: a table, some queries, a business
package, and an endpoint. The `users` package is the worked example — copy it.

1. **Migration.** Add `server/internal/store/migrations/000NN_thing.sql` with
   `-- +goose Up` and `-- +goose Down` sections. Add only what this change
   actually uses; a table nobody reads yet is a guess about the future.

2. **Queries.** Add `server/internal/store/queries/thing.sql`. Each query is
   named with a comment: `-- name: GetThing :one`.

3. **Generate.** `make gen`. Typed Go appears in `internal/store/gen`, checked
   against the schema. A column that does not exist is now a build failure.
   Commit the generated output.

4. **Business package.** Add `server/internal/thing/`. It declares its own
   `Store` interface naming only the queries it uses, returns its own types
   rather than database rows, validates its input, and returns `domain` errors.
   It must not import anything under `api/`.

5. **Tests.** Copy `internal/users/service_test.go`. Use
   `testutil.NewStore(t)` for a transactional store and `testutil.CreateUser`
   for fixtures. Test the rules, not the ORM.

6. **Handlers.** Add `server/internal/api/client/thing.go` (or `agent/`,
   `mgmt/`). A handler parses, calls one business method, and responds. It
   contains no logic and never writes an HTTP status directly.

7. **Route.** Add one line to the sub-router, and to `api/router.go` if it is a
   new area.

8. **Endpoint test.** Copy `internal/api/client/users_test.go`. It runs the real
   router, so middleware, status codes and the error envelope are covered too.

Then `make check`.

## Rules

**No file over 500 lines.** Enforced by `scripts/check-file-size.sh`, run by
`make test`. If a file is too long, split it by responsibility. Do not
special-case a file; if the limit is genuinely wrong, change
`CUCKOO_FILE_LINE_LIMIT` in the `Makefile` and say why.

**Errors are classified, never improvised.** Business code returns
`domain.NotFound(...)`, `domain.InvalidField(...)` and so on. One table —
`api/httpx/status.go` — decides what status each class becomes. No handler
writes a status code. An unclassified error is reported as a 500 with its cause
logged and nothing leaked to the caller.

**Every error needs a stable code and a message a person can read.** Clients
match on `code`; people read `message`. Both are part of the public protocol.

**The database is real in tests.** Do not mock the store to avoid running
Postgres. `make up` takes a few seconds. A test that skips because the database
is missing is not a test that passed — set `CI=true` to turn those skips into
failures.

**Comments explain why, not what.** The code says what it does. A comment earns
its place by recording a decision, a constraint, or a trap.

**No speculative code.** No placeholder packages, no unused fields, no
containers nothing talks to. Add it in the change that needs it.

## Conventions

- Identifiers are UUIDv7, exposed with a type prefix (`usr_`, `agt_`, `msg_`).
  Never expose a bare UUID.
- Timestamps are `timestamptz`. Postgres `now()` is the *transaction* start
  time, so it does not advance within one transaction — tests that need it to
  move must backdate a row first (see `TestUpdateProfileTouchesUpdatedAt`).
- Soft-delete anything a person can see the history of. Chat history must
  survive a departed account or a revoked agent.
- A write that can violate a constraint — a unique handle, say — runs inside
  `store.WithTx`. Postgres aborts a transaction on any error, and tests share
  one transaction per test; `WithTx` nests as a savepoint, so the failure is
  contained and the rest of the test (or request) can continue.
- Configuration comes from the environment only. Every problem is reported at
  once, so a broken deployment is fixed in one pass.

## Verification

There is no hosted CI on this repository, so verification runs on your machine.
Install the hooks once and most of it happens without you thinking about it:

```bash
make hooks
```

That points `core.hooksPath` at `scripts/hooks`, so the hooks live in the
repository and are versioned like everything else.

### The three gates

| When | What runs | Time |
|---|---|---|
| `git commit` | secrets, file size, gofmt, `go vet` | **~0.3s** |
| `git push` | build, full suite with `-race`, size, secrets | **~5s** |
| `make ci` | all of the above, plus a migrate-from-empty check, sqlc drift, and the linter | **~9s** |

Run `make ci` before opening a pull request. It is the closest thing to a real
CI run, and it checks two things nothing else does:

**It rebuilds the test database and migrates from empty.** Your development
database already has every table, so a broken migration keeps working locally
forever and fails on a self-hoster's first install. Migrating from nothing is
the only way to catch it.

**It regenerates the sqlc output and fails if it differs.** Edit a query, forget
`make gen`, and your Go and your schema disagree with nothing to notice.

### While you work

```bash
make watch                      # rerun tests on every save, no database
WATCH_FULL=1 make watch         # include database tests
make watch ARGS=./internal/users   # one package
```

### Rules the hooks enforce

**Never commit a credential.** This repository is public: a pushed secret is
scraped within minutes and deleting the commit does not un-leak it. Assume
anything committed is compromised and rotate it. `scripts/check-secrets.sh`
matches only high-confidence patterns — private keys, known token formats,
files that should never be tracked — so that it stays worth listening to. If it
flags something that genuinely is not a secret, put `ci:allow-secret` in a
comment on that line.

**A skipped test is not a passing test.** If Postgres is not running, the
database tests skip and the output looks almost identical to success. `pre-push`
refuses to run at all in that state; set `CI=true` to turn skips into failures
anywhere else.

### Bypassing

`git commit --no-verify` and `git push --no-verify` both work, and are the right
call for a work-in-progress commit on a private branch. They are not the right
call for anything reaching `main`. `make unhook` removes the hooks entirely.
