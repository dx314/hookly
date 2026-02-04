package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"time"
)

const telegramAPIURL = "https://api.telegram.org"

// TelegramNotifier sends notifications via Telegram.
type TelegramNotifier struct {
	botToken string
	chatID   string
	baseURL  string // For webhook detail links
	client   *http.Client
}

// NewTelegramNotifier creates a new Telegram notifier.
func NewTelegramNotifier(botToken, chatID, baseURL string) *TelegramNotifier {
	return &TelegramNotifier{
		botToken: botToken,
		chatID:   chatID,
		baseURL:  baseURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// NotifyDeliveryFailure sends a notification when a webhook fails permanently.
func (t *TelegramNotifier) NotifyDeliveryFailure(ctx context.Context, info WebhookInfo) error {
	message := fmt.Sprintf(
		`🚨 <b>Webhook Delivery Failed</b>

Endpoint: %s
Webhook ID: <code>%s</code>
Attempts: %d
Error: %s

<a href="%s/webhooks/%s">View Details</a>`,
		html.EscapeString(info.EndpointName),
		html.EscapeString(info.ID),
		info.Attempts,
		html.EscapeString(info.Error),
		t.baseURL,
		info.ID,
	)

	if err := t.sendMessage(ctx, message); err != nil {
		slog.Error("failed to send delivery failure notification",
			"webhook_id", info.ID,
			"error", err,
		)
		return err
	}

	slog.Info("sent delivery failure notification",
		"webhook_id", info.ID,
		"endpoint", info.EndpointName,
	)
	return nil
}

// NotifyDeadLetter sends a notification when a webhook becomes a dead letter.
func (t *TelegramNotifier) NotifyDeadLetter(ctx context.Context, info WebhookInfo) error {
	message := fmt.Sprintf(
		`⚠️ <b>Webhook Dead Letter</b>

Endpoint: %s
Webhook ID: <code>%s</code>
Received: %s

Webhook exceeded 7-day delivery window.

<a href="%s/webhooks/%s">View Details</a>`,
		html.EscapeString(info.EndpointName),
		html.EscapeString(info.ID),
		info.ReceivedAt.Format("2006-01-02 15:04:05 UTC"),
		t.baseURL,
		info.ID,
	)

	if err := t.sendMessage(ctx, message); err != nil {
		slog.Error("failed to send dead letter notification",
			"webhook_id", info.ID,
			"error", err,
		)
		return err
	}

	slog.Info("sent dead letter notification",
		"webhook_id", info.ID,
		"endpoint", info.EndpointName,
	)
	return nil
}

type telegramRequest struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

type telegramResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
}

func (t *TelegramNotifier) sendMessage(ctx context.Context, text string) error {
	url := fmt.Sprintf("%s/bot%s/sendMessage", telegramAPIURL, t.botToken)

	body, err := json.Marshal(telegramRequest{
		ChatID:    t.chatID,
		Text:      text,
		ParseMode: "HTML",
	})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	var result telegramResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if !result.OK {
		return fmt.Errorf("telegram error: %s", result.Description)
	}

	return nil
}
