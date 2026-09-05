// Package store owns all database access.
//
// Store embeds the sqlc-generated query set, so every generated method is
// available directly on it. Because the generated code accepts an interface
// rather than a concrete pool, the same Store can be backed by a connection
// pool in production or by a single transaction in a test — which is what makes
// the test suite both isolated and fast.
package store

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// Store is the handle every business package uses to reach the database.
//
// Exactly one of pool or tx is set. A pool-backed Store opens real
// transactions; a tx-backed Store — every test, and the inside of WithTx —
// nests with savepoints.
//
// A transaction is one connection, and a connection serves one query at a
// time, so a tx-backed Store serialises its callers through mu. That is what
// lets a test run the real server, whose socket handlers and workers do
// database work off the request goroutine, against a single rolled-back
// transaction. A pool-backed Store has no such lock; the pool is the lock.
type Store struct {
	*gen.Queries
	pool *pgxpool.Pool
	tx   pgx.Tx
	mu   *sync.Mutex
}

// Open connects to Postgres and verifies the connection.
func Open(ctx context.Context, databaseURL string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &Store{Queries: gen.New(pool), pool: pool}, nil
}

// NewFromDBTX builds a Store over any pgx executor — a pool, a connection, or a
// transaction. Tests use it to run against a transaction that is rolled back.
func NewFromDBTX(db gen.DBTX) *Store {
	tx, ok := db.(pgx.Tx)
	if !ok {
		return &Store{Queries: gen.New(db)}
	}
	mu := &sync.Mutex{}
	return &Store{Queries: gen.New(&lockedDBTX{db: tx, mu: mu}), tx: tx, mu: mu}
}

// Ping reports whether the database is reachable. Used by /healthz.
func (s *Store) Ping(ctx context.Context) error {
	if s.pool == nil {
		return errors.New("store: not backed by a connection pool")
	}
	return s.pool.Ping(ctx)
}

// Close releases the connection pool.
func (s *Store) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

// WithTx runs fn inside a database transaction, committing if it returns nil
// and rolling back otherwise.
//
// Inside an existing transaction it opens a savepoint instead. That matters
// more than it sounds: Postgres aborts a transaction on any error, including a
// constraint violation, and refuses every statement after it. A savepoint
// contains the failure — roll back to it and the enclosing transaction is
// usable again. Without this, a test that provokes one duplicate-handle
// conflict could never make another query, and a production request that
// tried an insert before doing something else would be in the same state.
//
// The Store handed to fn is scoped to the transaction; using the outer Store
// inside fn would silently escape it, so fn should only ever use its argument.
func (s *Store) WithTx(ctx context.Context, fn func(*Store) error) error {
	var (
		tx  pgx.Tx
		err error
	)
	switch {
	case s.tx != nil:
		if s.mu != nil {
			s.mu.Lock()
			defer s.mu.Unlock()
		}
		tx, err = s.tx.Begin(ctx) // pgx: a nested Begin is a savepoint
	case s.pool != nil:
		tx, err = s.pool.Begin(ctx)
	default:
		return errors.New("store: no transaction source")
	}
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		// Rollback after a successful commit is a no-op, so this is safe on
		// every path and guarantees the connection is never leaked on panic.
		_ = tx.Rollback(ctx)
	}()

	// Inside a savepoint the connection belongs to fn until it returns:
	// any other caller of the outer Store waits. The nested Store is
	// unlocked, since the lock is already held for it, and a nested WithTx
	// inside fn therefore takes no lock either.
	inner := &Store{Queries: gen.New(tx), tx: tx}
	if err := fn(inner); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// IsNoRows reports whether err means "the query matched nothing".
//
// Business packages use this to turn an empty result into a domain NotFound,
// which keeps pgx out of every package that reads from the database.
func IsNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

// IsUniqueViolation reports whether err is Postgres refusing a duplicate.
//
// Business packages turn this into a domain Conflict — "that handle is
// taken" — rather than a 500, and rely on the database rather than a
// check-then-insert, which has a race the database does not.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
