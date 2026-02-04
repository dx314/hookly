package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"connectrpc.com/connect"

	"hooks.dx314.com/internal/api/hookly/v1/hooklyv1connect"
	"hooks.dx314.com/internal/auth"
	"hooks.dx314.com/internal/config"
	"hooks.dx314.com/internal/db"
	"hooks.dx314.com/internal/notify"
	"hooks.dx314.com/internal/relay"
	"hooks.dx314.com/internal/server"
	"hooks.dx314.com/internal/service/edge"
	"hooks.dx314.com/internal/ui"
	"hooks.dx314.com/internal/webhook"
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
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Open database
	conn, err := db.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer conn.Close()

	queries := db.New(conn)
	secretManager := db.NewSecretManager(cfg.EncryptionKey)

	// Create relay connection manager
	connMgr := relay.NewConnectionManager()

	// Create notifier
	var notifier notify.Notifier = notify.NopNotifier{}
	if cfg.TelegramEnabled() {
		notifier = notify.NewTelegramNotifier(cfg.TelegramBotToken, cfg.TelegramChatID, cfg.BaseURL)
		slog.Info("telegram notifications enabled")
	}

	// Create server
	srv := server.New(fmt.Sprintf(":%d", cfg.Port))

	// Setup routes
	r := srv.Router()

	// Health check
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Webhook ingestion (no auth required)
	webhookHandler := webhook.NewHandler(queries, secretManager)
	r.Post("/h/{endpointID}", webhookHandler.ServeHTTP)

	// Authentication
	var sessionManager *auth.SessionManager
	var tokenManager *auth.TokenManager
	if cfg.GitHubAuthEnabled() {
		// Determine if running securely
		secure := strings.HasPrefix(cfg.BaseURL, "https://")
		redirectURI := cfg.BaseURL + "/auth/callback"

		githubClient := auth.NewGitHubClient(cfg.GitHubClientID, cfg.GitHubClientSecret, redirectURI)
		sessionManager = auth.NewSessionManager(queries, secure, "/")
		tokenManager = auth.NewTokenManager(queries)
		authorizer := auth.NewAuthorizer(githubClient, cfg.GitHubOrg, cfg.GitHubAllowedUsers)
		authHandlers := auth.NewHandlers(githubClient, sessionManager, authorizer, tokenManager)

		// Auth routes (no auth required)
		r.Get("/auth/login", authHandlers.Login)
		r.Get("/auth/callback", authHandlers.Callback)
		r.Post("/auth/logout", authHandlers.Logout)
		r.Get("/auth/me", authHandlers.Me)

		// CLI auth routes
		r.Get("/auth/cli/register", authHandlers.CLIRegister)
		r.Post("/auth/cli/authorize", authHandlers.CLIAuthorize)
		r.Post("/auth/token/revoke", authHandlers.RevokeToken)

		slog.Info("github auth enabled",
			"org_restriction", cfg.GitHubOrg != "",
			"user_restriction", len(cfg.GitHubAllowedUsers) > 0,
		)
	} else {
		slog.Warn("github auth disabled (GITHUB_CLIENT_ID/GITHUB_CLIENT_SECRET not set)")
	}

	// Relay service (ConnectRPC, uses bearer token auth)
	if tokenManager != nil {
		relayHandler := relay.NewHandler(tokenManager, connMgr, queries, notifier)
		path, handler := hooklyv1connect.NewRelayServiceHandler(relayHandler, connect.WithInterceptors())
		r.Mount(path, handler)
		slog.Info("relay service enabled")
	} else {
		slog.Warn("relay service disabled (GitHub auth not configured)")
	}

	// EdgeService (API for UI/MCP)
	edgeSvc := edge.New(queries, secretManager, connMgr, cfg)
	if sessionManager != nil {
		// With auth interceptor (supports both cookies and Bearer tokens)
		authInterceptor := server.NewAuthInterceptor(sessionManager, tokenManager)
		edgePath, edgeHandler := hooklyv1connect.NewEdgeServiceHandler(edgeSvc, connect.WithInterceptors(authInterceptor))
		r.Handle(edgePath+"*", edgeHandler)
		slog.Info("edge service enabled with auth")
	} else {
		// Without auth (development only)
		edgePath, edgeHandler := hooklyv1connect.NewEdgeServiceHandler(edgeSvc)
		r.Handle(edgePath+"*", edgeHandler)
		slog.Warn("edge service enabled WITHOUT auth (development mode)")
	}

	// UI handler
	uiHandler, err := ui.NewHandler("frontend/build")
	if err != nil {
		return fmt.Errorf("create ui handler: %w", err)
	}

	// Catch-all: Go vanity imports + SPA fallback
	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Query().Get("go-get") == "1" {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
<meta name="go-import" content="hooks.dx314.com git https://github.com/dx314/hookly">
<meta name="go-source" content="hooks.dx314.com https://github.com/dx314/hookly https://github.com/dx314/hookly/tree/main{/dir} https://github.com/dx314/hookly/blob/main{/dir}/{file}#L{line}">
</head>
<body>go get hooks.dx314.com/hookly</body>
</html>`)
			return
		}
		uiHandler.ServeHTTP(w, req)
	})
	slog.Info("ui handler enabled")

	// Start webhook dispatcher
	dispatcher := relay.NewDispatcher(queries, connMgr)
	go func() {
		if err := dispatcher.Run(ctx); err != nil && err != context.Canceled {
			slog.Error("dispatcher error", "error", err)
		}
	}()

	// Start webhook scheduler (dead-letter processing, cleanup)
	scheduler := webhook.NewScheduler(queries)
	scheduler.SetDeadLetterCallback(func(count int64) {
		slog.Warn("webhooks moved to dead letter", "count", count)
		// Send dead letter notifications
		go sendDeadLetterNotifications(context.Background(), queries, notifier)
	})
	go func() {
		if err := scheduler.Start(ctx); err != nil && err != context.Canceled {
			slog.Error("scheduler error", "error", err)
		}
	}()

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	slog.Info("edge-gateway started",
		"port", cfg.Port,
		"base_url", cfg.BaseURL,
		"github_auth", cfg.GitHubAuthEnabled(),
		"telegram", cfg.TelegramEnabled(),
	)

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case sig := <-sigCh:
		slog.Info("received shutdown signal", "signal", sig)
	}

	// Graceful shutdown
	cancel() // Stop dispatcher
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}

	slog.Info("edge-gateway stopped")
	return nil
}

// sendDeadLetterNotifications sends notifications for recently dead-lettered webhooks.
func sendDeadLetterNotifications(ctx context.Context, queries *db.Queries, notifier notify.Notifier) {
	// Get unnotified dead letters (limit to prevent spam)
	rows, err := queries.GetUnnotifiedDeadLetters(ctx, 50)
	if err != nil {
		slog.Error("failed to get dead letter webhooks", "error", err)
		return
	}

	for _, row := range rows {
		// Parse received_at time
		receivedAt, _ := time.Parse("2006-01-02 15:04:05", row.ReceivedAt)

		info := notify.WebhookInfo{
			ID:             row.ID,
			EndpointID:     row.EndpointID,
			EndpointName:   row.EndpointName,
			DestinationURL: row.EndpointDestinationUrl,
			Attempts:       int(row.Attempts),
			ReceivedAt:     receivedAt,
		}

		if err := notifier.NotifyDeadLetter(ctx, info); err != nil {
			// Log but continue with other notifications
			continue
		}

		// Mark as notified
		if err := queries.MarkNotificationSent(ctx, row.ID); err != nil {
			slog.Error("failed to mark notification sent", "webhook_id", row.ID, "error", err)
		}
	}
}
