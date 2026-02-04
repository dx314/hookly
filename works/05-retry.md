# Work Order 05: Retry Logic

**Swimlane:** Backend
**Status:** DONE
**Dependencies:** 01-database, 03-relay, 04-home-hub

---

## Objective

Implement retry logic with exponential backoff and dead-letter handling.

---

## Tasks

### Retry Scheduler
- [x] Create `internal/webhook/scheduler.go`:
  - Background goroutine
  - Poll for webhooks needing retry (handled by dispatcher with updated query)
  - Calculate next retry time
  - Push to relay for delivery

### Backoff Calculation
- [x] Create `internal/webhook/backoff.go`:
  ```go
  func NextRetryDelay(attempts int) time.Duration {
    // 1s, 2s, 4s, 8s, 16s, 32s, 64s, 128s, 256s, 512s, 1024s, 2048s, 3600s (cap)
    delay := time.Second * time.Duration(1<<attempts)
    if delay > time.Hour {
      delay = time.Hour
    }
    return delay
  }

  func ShouldRetry(webhook) bool {
    // Check if enough time has passed since last attempt
    nextRetry := webhook.LastAttemptAt.Add(NextRetryDelay(webhook.Attempts))
    return time.Now().After(nextRetry)
  }
  ```

### Status Transitions
- [x] Document state machine:
  ```
  pending → delivered (on 2xx)
  pending → failed (on 4xx)
  pending → pending (on 5xx, increment attempts)
  pending → dead_letter (after 7 days)
  ```

### Dead Letter Processing
- [x] Consolidated into `internal/webhook/scheduler.go`:
  - Background job (run every hour)
  - Find webhooks: status=pending AND received_at < 7 days ago
  - Update status to dead_letter
  - Trigger notification via callback (actual notification in work order 10)

### Retention Cleanup
- [x] Consolidated into `internal/webhook/scheduler.go`:
  - Background job (run every hour)
  - Delete webhooks: status=delivered AND delivered_at < 7 days ago
  - Delete webhooks: status=failed AND last_attempt_at < 7 days ago
  - Delete webhooks: status=dead_letter AND received_at < 14 days ago

### Webhook Status Update
- [x] On ACK received from home-hub (in `internal/relay/handler.go`):
  - Success → status=delivered, delivered_at=now (`MarkWebhookDelivered`)
  - Permanent failure (4xx) → status=failed, error_message=... (`MarkWebhookFailed`)
  - Transient failure (5xx) → attempts++, last_attempt_at=now, stay pending (`RecordWebhookAttempt`)

### In-Order Delivery
- [x] Ensure webhooks to same endpoint delivered in order
- [x] Don't deliver webhook N+1 until webhook N is delivered/failed
- [x] Query: get oldest pending webhook per endpoint (updated `GetPendingWebhooks` query)

---

## Acceptance Criteria

- [x] Failed delivery (5xx) retries after backoff
- [x] Retry intervals: 1s, 2s, 4s... max 1 hour
- [x] 4xx response stops retrying immediately
- [x] Webhooks pending >7 days → dead_letter
- [x] Delivered webhooks deleted after 7 days
- [x] In-order delivery per endpoint maintained
- [x] Dead letter triggers notification (callback wired up, notification implementation in work order 10)

---

## Implementation Notes

- Consolidated deadletter.go and cleanup.go into scheduler.go since they run on the same hourly timer
- Backoff timing is handled in SQL query (`GetPendingWebhooks`) for database-level enforcement
- Added backoff_test.go with unit tests for delay calculations
- Scheduler starts automatically with edge-gateway

## Files Modified/Created

- `internal/webhook/backoff.go` - Backoff delay calculation functions
- `internal/webhook/backoff_test.go` - Unit tests for backoff
- `internal/webhook/scheduler.go` - Background job scheduler (dead-letter, cleanup)
- `internal/relay/handler.go` - Updated ACK handling with proper status transitions
- `sql/queries/webhooks.sql` - Added/updated queries for retry, dead-letter, cleanup
- `cmd/edge-gateway/main.go` - Added scheduler startup

---

## Notes

- Use `time.Ticker` for background jobs ✓
- Consider edge restart: resume retry schedule from database state ✓ (query uses database timestamps)
- Log retry attempts with webhook ID and attempt number ✓
