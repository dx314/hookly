package notify

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"hooks.dx314.com/internal/db"
)

// SecretManager provides encryption/decryption for secrets.
type SecretManager interface {
	DecryptSecret(encrypted []byte) (string, error)
}

// UserNotifier is a notifier that checks per-user Telegram config first,
// then falls back to a global notifier.
type UserNotifier struct {
	queries       *db.Queries
	secretManager SecretManager
	globalConfig  Notifier
	baseURL       string
}

// NewUserNotifier creates a new user notifier.
// globalConfig is the fallback notifier when user config is not set.
func NewUserNotifier(queries *db.Queries, secretManager SecretManager, globalConfig Notifier, baseURL string) *UserNotifier {
	if globalConfig == nil {
		globalConfig = NopNotifier{}
	}
	return &UserNotifier{
		queries:       queries,
		secretManager: secretManager,
		globalConfig:  globalConfig,
		baseURL:       baseURL,
	}
}

// NotifyDeliveryFailure sends a notification when a webhook fails permanently.
// It first checks for per-user Telegram config, then falls back to global.
func (u *UserNotifier) NotifyDeliveryFailure(ctx context.Context, info WebhookInfo) error {
	notifier := u.getNotifierForEndpoint(ctx, info.EndpointID)
	return notifier.NotifyDeliveryFailure(ctx, info)
}

// NotifyDeadLetter sends a notification when a webhook becomes a dead letter.
// It first checks for per-user Telegram config, then falls back to global.
func (u *UserNotifier) NotifyDeadLetter(ctx context.Context, info WebhookInfo) error {
	notifier := u.getNotifierForEndpoint(ctx, info.EndpointID)
	return notifier.NotifyDeadLetter(ctx, info)
}

// getNotifierForEndpoint returns the appropriate notifier for an endpoint.
// It checks if the endpoint owner has Telegram configured and enabled.
func (u *UserNotifier) getNotifierForEndpoint(ctx context.Context, endpointID string) Notifier {
	// Try to get user's Telegram config via the endpoint
	config, err := u.queries.GetEndpointOwnerTelegramConfig(ctx, endpointID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Debug("failed to get endpoint owner telegram config", "endpoint_id", endpointID, "error", err)
		}
		// Fall back to global notifier
		return u.globalConfig
	}

	// Check if user has Telegram enabled with valid config
	if config.TelegramEnabled == 0 || len(config.TelegramBotTokenEncrypted) == 0 || !config.TelegramChatID.Valid {
		// User hasn't configured Telegram, use global
		return u.globalConfig
	}

	// Decrypt the bot token
	botToken, err := u.secretManager.DecryptSecret(config.TelegramBotTokenEncrypted)
	if err != nil {
		slog.Error("failed to decrypt user telegram token", "user_id", config.UserID, "error", err)
		return u.globalConfig
	}

	// Create a new TelegramNotifier for this user
	slog.Debug("using per-user telegram notifier",
		"user_id", config.UserID,
		"endpoint_id", endpointID,
	)
	return NewTelegramNotifier(botToken, config.TelegramChatID.String, u.baseURL)
}
