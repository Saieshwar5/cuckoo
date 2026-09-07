package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Migrations are embedded in the binary so a deployment is one file, and a
// deployment upgrades by replacing that file and restarting.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate brings the database up to the latest schema.
//
// It runs at startup rather than as a separate deploy step: with a single
// server process there is no window in which code and schema disagree, and
// a deploy cannot forget to run it.
func Migrate(ctx context.Context, databaseURL string, log *slog.Logger) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open database for migration: %w", err)
	}
	defer func() { _ = db.Close() }()

	goose.SetBaseFS(migrationsFS)
	goose.SetLogger(gooseLogger{log})
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set migration dialect: %w", err)
	}

	// Hold an advisory lock so that two processes starting together — a second
	// server instance, or several test binaries — cannot apply the same
	// migration twice. Waiters block until the first finishes, then find
	// nothing left to do.
	unlock, err := lockForMigration(ctx, db)
	if err != nil {
		return err
	}
	defer unlock()

	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	version, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	log.Info("database schema ready", "version", version)
	return nil
}

// migrationLockKey is an arbitrary constant identifying the migration lock.
// Any process using this database must use the same number for the lock to
// mean anything.
const migrationLockKey int64 = 4823

// lockForMigration takes a Postgres advisory lock and returns the release
// function. The lock is held on one dedicated connection, because an advisory
// lock belongs to the session that took it.
func lockForMigration(ctx context.Context, db *sql.DB) (func(), error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire connection for migration lock: %w", err)
	}

	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("take migration lock: %w", err)
	}

	return func() {
		// Released explicitly rather than by closing the connection, so the
		// lock is gone the moment migrations finish.
		_, _ = conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", migrationLockKey)
		_ = conn.Close()
	}, nil
}

// gooseLogger routes goose's output through slog so migration output is
// structured like everything else.
type gooseLogger struct{ log *slog.Logger }

func (g gooseLogger) Printf(format string, v ...any) {
	g.log.Debug("goose: " + fmt.Sprintf(format, v...))
}

func (g gooseLogger) Fatalf(format string, v ...any) {
	// goose calls Fatalf for migration failures; the error is also returned to
	// the caller, which is what actually stops startup.
	g.log.Error("goose: " + fmt.Sprintf(format, v...))
}

// stdlib is imported for its side effect: registering the "pgx" database/sql
// driver that goose needs. The rest of the server uses pgx directly.
var _ = stdlib.GetDefaultDriver
