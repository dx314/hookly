package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Migrate applies database migrations using goose.
func Migrate(ctx context.Context, db *sql.DB) error {
	slog.Info("running database migrations")

	// Enable foreign keys
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode = WAL"); err != nil {
		return fmt.Errorf("enable WAL mode: %w", err)
	}

	// Configure goose
	goose.SetBaseFS(migrations)
	goose.SetLogger(goose.NopLogger())

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}

	// Auto-baseline existing databases (before goose was added)
	if err := autoBaseline(ctx, db); err != nil {
		return fmt.Errorf("auto-baseline: %w", err)
	}

	// Run migrations
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	// Adopt rows written by a binary that predates multi-destination fan-out
	if err := adoptLegacyRows(ctx, db); err != nil {
		return fmt.Errorf("adopt legacy rows: %w", err)
	}

	// Log current version
	version, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return fmt.Errorf("get version: %w", err)
	}

	slog.Info("database migrations complete", "version", version)
	return nil
}

// MigrateTo migrates a database up to a specific version (used by tests to
// build a database as an older release left it).
func MigrateTo(ctx context.Context, db *sql.DB, version int64) error {
	goose.SetBaseFS(migrations)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}
	return goose.UpToContext(ctx, db, "migrations", version)
}

// MigrateStatus returns the current migration status.
func MigrateStatus(ctx context.Context, db *sql.DB) error {
	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}
	return goose.StatusContext(ctx, db, "migrations")
}

// autoBaseline detects existing databases (created before goose) and marks
// migrations 1-2 as applied so goose doesn't try to re-run them.
func autoBaseline(ctx context.Context, db *sql.DB) error {
	// Check if goose table exists
	var gooseExists int
	err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='goose_db_version'",
	).Scan(&gooseExists)
	if err != nil {
		return fmt.Errorf("check goose table: %w", err)
	}
	if gooseExists > 0 {
		return nil // Already using goose
	}

	// Check if this is an existing database (has endpoints table)
	var endpointsExist int
	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='endpoints'",
	).Scan(&endpointsExist)
	if err != nil {
		return fmt.Errorf("check endpoints table: %w", err)
	}
	if endpointsExist == 0 {
		return nil // Fresh database, no baseline needed
	}

	slog.Info("detected existing database, applying baseline")

	// Create goose version table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE goose_db_version (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			version_id INTEGER NOT NULL,
			is_applied INTEGER NOT NULL,
			tstamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("create goose table: %w", err)
	}

	// Mark migrations 1-2 as applied (they were applied before goose)
	_, err = db.ExecContext(ctx, `
		INSERT INTO goose_db_version (version_id, is_applied) VALUES (0, 1);
		INSERT INTO goose_db_version (version_id, is_applied) VALUES (1, 1);
		INSERT INTO goose_db_version (version_id, is_applied) VALUES (2, 1);
	`)
	if err != nil {
		return fmt.Errorf("insert baseline: %w", err)
	}

	slog.Info("database baselined to version 2")
	return nil
}

// adoptLegacyRows keeps a rollback to a pre-fan-out binary safe. Migration 007
// back-fills destinations and deliveries once; anything an older binary writes
// afterwards (endpoints without destinations, pending webhooks without
// deliveries) is picked up here on the next start. Idempotent, and a no-op in
// normal operation: endpoints always have a destination and new webhooks always
// get deliveries. Finished webhooks are never re-fanned out.
func adoptLegacyRows(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		INSERT INTO destinations (id, endpoint_id, name, url, enabled, position, created_at, updated_at)
		SELECT 'dst_' || lower(hex(randomblob(12))), e.id, ?, e.destination_url, 1, 0, e.created_at, e.updated_at
		FROM endpoints e
		WHERE NOT EXISTS (SELECT 1 FROM destinations d WHERE d.endpoint_id = e.id)`,
		DefaultDestinationName)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		slog.Info("created destinations from legacy destination_url", "endpoints", n)
	}

	res, err = tx.ExecContext(ctx, `
		INSERT INTO deliveries (id, webhook_id, destination_id, status, attempts, last_attempt_at,
		                        error_message, notification_sent, created_at)
		SELECT 'dl_' || lower(hex(randomblob(12))), w.id,
		       (SELECT d.id FROM destinations d WHERE d.endpoint_id = w.endpoint_id
		        ORDER BY d.position ASC, d.created_at ASC, d.id ASC LIMIT 1),
		       'pending', w.attempts, w.last_attempt_at, w.error_message, w.notification_sent, w.received_at
		FROM webhooks w
		WHERE w.status = 'pending'
		  AND NOT EXISTS (SELECT 1 FROM deliveries dl WHERE dl.webhook_id = w.id)
		ORDER BY w.received_at ASC, w.rowid ASC`)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		slog.Info("created deliveries for pending webhooks without any", "deliveries", n)
	}

	return tx.Commit()
}
