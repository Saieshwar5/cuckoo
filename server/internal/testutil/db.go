// Package testutil provides the scaffolding every test in the server uses.
//
// The central idea is that tests run against a real Postgres, never a mock:
// the hard bugs in a messaging system live in the database — ordering,
// uniqueness, partial updates, delivery state — and a mock will happily agree
// with whatever the code believes. Isolation comes from running each test in a
// transaction that is rolled back, which is both stricter than cleaning up
// afterwards and considerably faster.
package testutil

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Saieshwar5/cuckoo/server/internal/store"
)

// defaultTestDatabaseURL matches what `make up` creates. Tests never touch the
// development database, so a test run can never destroy data you are looking
// at in the app.
// The credentials here are the throwaway ones `make up` creates for a local
// container. Nothing outside a developer's machine ever accepts them.
//
//nolint:gosec // G101: local development database, not a real credential
const defaultTestDatabaseURL = "postgres://cuckoo:cuckoo@localhost:5433/cuckoo_test?sslmode=disable"

var (
	poolOnce sync.Once
	pool     *pgxpool.Pool
	poolErr  error
)

// NewStore returns a Store bound to a transaction that is rolled back when the
// test finishes.
//
// Tests are therefore independent regardless of order, leave nothing behind
// even when they fail, and can run in parallel.
func NewStore(t *testing.T) *store.Store {
	t.Helper()
	db, _ := NewStoreTx(t)
	return db
}

// NewStoreTx returns the same transactional Store along with the transaction
// itself, for tests that need raw SQL to set up a state the application cannot
// produce on its own — backdating a timestamp, or corrupting a row to check
// that recovery works.
func NewStoreTx(t *testing.T) (*store.Store, pgx.Tx) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping database test in -short mode")
	}

	tx, err := sharedPool(t).Begin(context.Background())
	if err != nil {
		t.Fatalf("testutil: begin transaction: %v", err)
	}
	t.Cleanup(func() {
		// The rollback discards everything the test wrote. An error here means
		// the connection is already gone, which the pool handles.
		_ = tx.Rollback(context.Background())
	})

	return store.NewFromDBTX(tx), tx
}

// sharedPool opens the test database once per test binary and migrates it.
func sharedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	poolOnce.Do(func() {
		ctx := context.Background()
		url := testDatabaseURL()

		// Migrations are silent in tests; a failure is reported by the error.
		quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
		if poolErr = store.Migrate(ctx, url, quiet); poolErr != nil {
			return
		}

		// The pool is opened directly rather than through store.Open so that
		// store's public API is not widened for the benefit of tests.
		pool, poolErr = pgxpool.New(ctx, url)
		if poolErr != nil {
			return
		}
		poolErr = pool.Ping(ctx)
	})

	if poolErr != nil {
		// In CI an unreachable database is a broken build, not a reason to
		// quietly pass: a suite that skips everything looks identical to a
		// suite that succeeds.
		if os.Getenv("CI") != "" {
			t.Fatalf("testutil: test database unavailable in CI: %v", poolErr)
		}
		t.Skipf("testutil: test database unavailable (run `make up`): %v", poolErr)
	}

	return pool
}

func testDatabaseURL() string {
	if url := os.Getenv("CUCKOO_TEST_DATABASE_URL"); url != "" {
		return url
	}
	return defaultTestDatabaseURL
}
