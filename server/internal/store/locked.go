package store

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// lockedDBTX serialises access to a transaction. Each call holds the lock
// for as long as the connection is busy: an Exec until it returns, a Query
// until its rows are closed, a QueryRow until its row is scanned.
type lockedDBTX struct {
	db pgx.Tx
	mu *sync.Mutex
}

func (l *lockedDBTX) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.db.Exec(ctx, sql, args...)
}

func (l *lockedDBTX) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	l.mu.Lock()
	rows, err := l.db.Query(ctx, sql, args...)
	if err != nil {
		l.mu.Unlock()
		return nil, err
	}
	return &lockedRows{Rows: rows, unlock: l.mu.Unlock}, nil
}

func (l *lockedDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	l.mu.Lock()
	return &lockedRow{row: l.db.QueryRow(ctx, sql, args...), unlock: l.mu.Unlock}
}

// lockedRows releases the connection when the caller is done reading, which
// sqlc's generated code always does with a deferred Close.
type lockedRows struct {
	pgx.Rows
	once   sync.Once
	unlock func()
}

func (r *lockedRows) Close() {
	r.Rows.Close()
	r.once.Do(r.unlock)
}

// lockedRow releases the connection once the single row has been read.
type lockedRow struct {
	row    pgx.Row
	once   sync.Once
	unlock func()
}

func (r *lockedRow) Scan(dest ...any) error {
	defer r.once.Do(r.unlock)
	return r.row.Scan(dest...)
}
