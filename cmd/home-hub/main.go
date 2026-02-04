package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"hookly/internal/config"
	"hookly/internal/relay"
)

func main() {
	// Setup structured logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load config
	cfg, err := config.LoadHome()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	slog.Info("home-hub starting",
		"edge_url", cfg.EdgeURL,
		"hub_id", cfg.HubID,
	)

	// Create relay client
	client := relay.NewClient(cfg.EdgeURL, cfg.HomeHubSecret, cfg.HubID)

	// Run client in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Run(ctx)
	}()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			return fmt.Errorf("client error: %w", err)
		}
	case sig := <-sigCh:
		slog.Info("received shutdown signal", "signal", sig)
	}

	// Graceful shutdown
	cancel()
	slog.Info("home-hub stopped")
	return nil
}
