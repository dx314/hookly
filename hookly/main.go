package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"hooks.dx314.com/internal/config"
	"hooks.dx314.com/internal/relay"
)

const version = "0.1.0"

func main() {
	// Setup structured logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			runInit()
			return
		case "version", "-v", "--version":
			fmt.Printf("hookly version %s\n", version)
			return
		case "help", "-h", "--help":
			printHelp()
			return
		}
	}

	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load config from hookly.yaml
	cfg, err := config.LoadHooklyYAML("hookly.yaml")
	if err != nil {
		return fmt.Errorf("load config: %w\n\nRun 'hookly init' to create a hookly.yaml file", err)
	}

	slog.Info("hookly starting",
		"edge_url", cfg.EdgeURL,
		"hub_id", cfg.HubID,
		"endpoints", len(cfg.Endpoints),
	)

	// Create relay client
	client := relay.NewClient(cfg)

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
	slog.Info("hookly stopped")
	return nil
}

func runInit() {
	// Check if file already exists
	if _, err := os.Stat("hookly.yaml"); err == nil {
		fmt.Println("hookly.yaml already exists")
		os.Exit(1)
	}

	// Write example config
	if err := os.WriteFile("hookly.yaml", []byte(config.ExampleYAML()), 0644); err != nil {
		fmt.Printf("Error creating hookly.yaml: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Created hookly.yaml")
	fmt.Println("")
	fmt.Println("Edit the file to configure:")
	fmt.Println("  - edge_url: Your hookly edge server URL")
	fmt.Println("  - secret: Your home hub secret")
	fmt.Println("  - hub_id: A unique identifier for this hub")
	fmt.Println("  - endpoints: The endpoint IDs to handle")
}

func printHelp() {
	fmt.Println("hookly - Webhook relay client")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  hookly          Start the relay (reads hookly.yaml from current directory)")
	fmt.Println("  hookly init     Create an example hookly.yaml file")
	fmt.Println("  hookly version  Print version information")
	fmt.Println("  hookly help     Show this help message")
	fmt.Println("")
	fmt.Println("Configuration:")
	fmt.Println("  hookly reads configuration from hookly.yaml in the current directory.")
	fmt.Println("  Run 'hookly init' to create a template configuration file.")
	fmt.Println("")
	fmt.Println("Example hookly.yaml:")
	fmt.Println("")
	fmt.Print(config.ExampleYAML())
}
