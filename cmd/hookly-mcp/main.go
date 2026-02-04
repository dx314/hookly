package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"hooks.dx314.com/internal/crypto"
	"hooks.dx314.com/internal/db"
	"hooks.dx314.com/internal/mcp"

	"github.com/joho/godotenv"
)

func main() {
	// Setup logging to stderr (stdout is for MCP protocol)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn, // Quiet by default
	})))

	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	// Load .env file if present
	_ = godotenv.Load()

	// Get required configuration
	databasePath := os.Getenv("DATABASE_PATH")
	if databasePath == "" {
		databasePath = "./hookly.db"
	}

	keyHex := os.Getenv("ENCRYPTION_KEY")
	if keyHex == "" {
		return errors.New("ENCRYPTION_KEY is required")
	}
	key, err := crypto.ParseKey(keyHex)
	if err != nil {
		return fmt.Errorf("invalid ENCRYPTION_KEY: %w", err)
	}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	// Open database
	conn, err := db.Open(ctx, databasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer conn.Close()

	queries := db.New(conn)
	secretManager := db.NewSecretManager(key)

	// Create and run MCP server
	server := mcp.NewServer(queries, secretManager, baseURL)
	return server.ServeStdio()
}
