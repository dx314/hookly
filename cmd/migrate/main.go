// Command migrate provides CLI access to database migrations.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/mattn/go-sqlite3"
	"github.com/pressly/goose/v3"

	"hooks.dx314.com/internal/db"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: migrate <command> [database]")
		fmt.Println("Commands: up, down, status, baseline")
		fmt.Println("Database path from DATABASE_PATH env or argument (default: ./hookly.db)")
		os.Exit(1)
	}

	command := os.Args[1]

	// Get database path: arg > env > default
	dbPath := "./hookly.db"
	if envPath := os.Getenv("DATABASE_PATH"); envPath != "" {
		dbPath = envPath
	}
	if len(os.Args) >= 3 {
		dbPath = os.Args[2]
	}

	database, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	ctx := context.Background()

	switch command {
	case "baseline":
		// Mark migrations 1-2 as applied for existing databases
		if err := baseline(ctx, database); err != nil {
			fmt.Fprintf(os.Stderr, "Baseline failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Database baselined to version 2")

	case "up":
		if err := db.Migrate(ctx, database); err != nil {
			fmt.Fprintf(os.Stderr, "Migration failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Migrations applied successfully")

	case "down":
		goose.SetBaseFS(nil) // Use filesystem directly for down
		if err := goose.SetDialect("sqlite3"); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to set dialect: %v\n", err)
			os.Exit(1)
		}
		if err := goose.Down(database, "internal/db/migrations"); err != nil {
			fmt.Fprintf(os.Stderr, "Migration down failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Migration rolled back")

	case "status":
		if err := db.MigrateStatus(ctx, database); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get status: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		os.Exit(1)
	}
}

// baseline marks migrations 1-2 as applied for existing production databases.
// This should only be run once on databases that existed before goose was added.
func baseline(ctx context.Context, database *sql.DB) error {
	// Check if endpoints table exists (indicates existing database)
	var tableName string
	err := database.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='endpoints'").Scan(&tableName)
	if err != nil {
		return fmt.Errorf("database doesn't appear to have existing tables: %w", err)
	}

	// Create goose version table if needed
	_, err = database.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS goose_db_version (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			version_id INTEGER NOT NULL,
			is_applied INTEGER NOT NULL,
			tstamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("create goose table: %w", err)
	}

	// Check if already baselined
	var count int
	err = database.QueryRowContext(ctx, "SELECT COUNT(*) FROM goose_db_version WHERE version_id > 0").Scan(&count)
	if err != nil {
		return fmt.Errorf("check baseline: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("database already has migrations applied")
	}

	// Insert baseline versions (1 and 2 were applied before goose)
	_, err = database.ExecContext(ctx, `
		INSERT INTO goose_db_version (version_id, is_applied) VALUES (1, 1);
		INSERT INTO goose_db_version (version_id, is_applied) VALUES (2, 1);
	`)
	if err != nil {
		return fmt.Errorf("insert baseline: %w", err)
	}

	return nil
}
