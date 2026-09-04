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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cuckoo-chat/cuckoo/server/internal/store/gen"
)

// Store is the handle every business package uses to reach the database.
type Store struct {
	*gen.Queries
	pool *pgxpool.Pool // nil when the Store is backed by a transaction
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
	return &Store{Queries: gen.New(db)}
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
// The Store handed to fn is scoped to the transaction; using the outer Store
// inside fn would silently escape it, so fn should only ever use its argument.
func (s *Store) WithTx(ctx context.Context, fn func(*Store) error) error {
	if s.pool == nil {
		// Already inside a transaction (a test, or a nested call). Postgres
		// savepoints would let this nest, but no caller needs that yet and
		// pretending to start a transaction we cannot commit would be worse.
		return fn(s)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		// Rollback after a successful commit is a no-op, so this is safe on
		// every path and guarantees the connection is never leaked on panic.
		_ = tx.Rollback(ctx)
	}()

	if err := fn(NewFromDBTX(tx)); err != nil {
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
