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
- Configuration comes from the environment only. Every problem is reported at
  once, so a broken deployment is fixed in one pass.

## Before committing

There is no CI on this repository, so this command is the only gate. Run it
every time.

```bash
make up         # if Postgres and Redis are not already running
make check      # file-size guard, linter, and the full test suite
```

`make check` must be green before you commit. If the database is not running,
database tests skip rather than fail — which is convenient locally and
dangerous silently, so run `make up` first and check the output says tests
passed rather than that they were skipped.

A GitHub Actions workflow running exactly these steps is easy to add if Actions
becomes available; nothing in the project depends on its absence.
