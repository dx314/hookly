package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"log/slog"
)

//go:embed schema.sql
var schemaSQL string

// Migrate applies database migrations.
// For simplicity, this runs the full schema on every startup.
// SQLite's IF NOT EXISTS makes this safe.
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

	// Run schema
	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}

	// Migration: Add notification_sent column if missing
	if err := migrateNotificationSent(ctx, db); err != nil {
		return fmt.Errorf("migrate notification_sent: %w", err)
	}

	slog.Info("database migrations complete")
	return nil
}

// migrateNotificationSent adds the notification_sent column to existing databases.
func migrateNotificationSent(ctx context.Context, db *sql.DB) error {
	// Check if column exists
	rows, err := db.QueryContext(ctx, "PRAGMA table_info(webhooks)")
	if err != nil {
		return err
	}
	defer rows.Close()

	var hasColumn bool
	for rows.Next() {
		var cid int
		var name, typeName string
		var notNull, pk int
		var dfltValue any
		if err := rows.Scan(&cid, &name, &typeName, &notNull, &dfltValue, &pk); err != nil {
			return err
		}
		if name == "notification_sent" {
			hasColumn = true
			break
		}
	}

	if !hasColumn {
		slog.Info("adding notification_sent column to webhooks table")
		_, err := db.ExecContext(ctx, "ALTER TABLE webhooks ADD COLUMN notification_sent INTEGER NOT NULL DEFAULT 0")
		if err != nil {
			return err
		}
	}

	return nil
}
