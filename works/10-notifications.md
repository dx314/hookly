# Work Order 10: Notifications

**Swimlane:** Integration
**Status:** DONE
**Dependencies:** 05-retry

---

## Objective

Implement Telegram notifications for delivery failures.

---

## Tasks

### Telegram Client
- [x] Create `internal/notify/telegram.go`:
  ```go
  type TelegramNotifier struct {
    botToken string
    chatID   string
    client   *http.Client
  }

  func (t *TelegramNotifier) SendMessage(text string) error
  ```

### Telegram API
- [x] Use HTTP API: `https://api.telegram.org/bot{token}/sendMessage`
- [x] Request body:
  ```json
  {
    "chat_id": "...",
    "text": "...",
    "parse_mode": "HTML"
  }
  ```

### Notification Interface
- [x] Create `internal/notify/notifier.go`:
  ```go
  type Notifier interface {
    NotifyDeliveryFailure(endpoint, webhook, error) error
    NotifyDeadLetter(endpoint, webhook) error
  }
  ```

### Message Templates
- [x] Delivery failure message:
  ```
  🚨 <b>Webhook Delivery Failed</b>

  Endpoint: {name}
  Webhook ID: {id}
  Attempts: {attempts}
  Error: {error}

  <a href="{base_url}/webhooks/{id}">View Details</a>
  ```

- [x] Dead letter message:
  ```
  ⚠️ <b>Webhook Dead Letter</b>

  Endpoint: {name}
  Webhook ID: {id}
  Received: {received_at}

  Webhook exceeded 7-day delivery window.

  <a href="{base_url}/webhooks/{id}">View Details</a>
  ```

### Integration Points
- [x] Call notifier on permanent failure (4xx after retries)
- [x] Call notifier on dead letter transition
- [x] Don't notify on every retry (too noisy)

### Rate Limiting
- [x] Limit notifications: max 1 per webhook
- [x] Track notification_sent flag on webhook

### Graceful Degradation
- [x] If Telegram fails, log error but don't crash
- [x] If not configured, skip silently

---

## Acceptance Criteria

- [x] Telegram message sent on delivery failure
- [x] Telegram message sent on dead letter
- [x] Message includes endpoint name and error
- [x] Message includes link to webhook details
- [x] No notification spam (1 per webhook)
- [x] Handles Telegram API errors gracefully
- [x] Works without Telegram configured

---

## Notes

- Test with BotFather: https://t.me/BotFather
- Get chat ID by messaging bot and checking getUpdates

## Implementation Summary

Files created/modified:
- `internal/notify/notifier.go` - Notifier interface and NopNotifier
- `internal/notify/telegram.go` - TelegramNotifier implementation
- `internal/relay/handler.go` - Added notifier support, sends failure notifications
- `cmd/edge-gateway/main.go` - Wires up notifier, dead letter notifications
- `sql/schema.sql` - Added notification_sent column
- `sql/queries/webhooks.sql` - Added notification queries
- `internal/db/migrations.go` - Migration for notification_sent column
