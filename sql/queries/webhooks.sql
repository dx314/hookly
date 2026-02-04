-- name: CreateWebhook :one
INSERT INTO webhooks (id, endpoint_id, received_at, headers, payload, signature_valid, status, attempts)
VALUES (?, ?, datetime('now'), ?, ?, ?, 'pending', 0)
RETURNING *;

-- name: GetWebhook :one
SELECT * FROM webhooks WHERE id = ?;

-- name: ListWebhooks :many
SELECT * FROM webhooks
WHERE (sqlc.narg('endpoint_id') IS NULL OR endpoint_id = sqlc.narg('endpoint_id'))
  AND (sqlc.narg('status') IS NULL OR status = sqlc.narg('status'))
ORDER BY received_at DESC
LIMIT ? OFFSET ?;

-- name: CountWebhooks :one
SELECT COUNT(*) FROM webhooks
WHERE (sqlc.narg('endpoint_id') IS NULL OR endpoint_id = sqlc.narg('endpoint_id'))
  AND (sqlc.narg('status') IS NULL OR status = sqlc.narg('status'));

-- name: MarkWebhookDelivered :one
-- Mark a webhook as successfully delivered.
UPDATE webhooks
SET status = 'delivered',
    attempts = attempts + 1,
    last_attempt_at = datetime('now'),
    delivered_at = datetime('now'),
    error_message = NULL
WHERE id = ?
RETURNING *;

-- name: MarkWebhookFailed :one
-- Mark a webhook as permanently failed (4xx response).
UPDATE webhooks
SET status = 'failed',
    attempts = attempts + 1,
    last_attempt_at = datetime('now'),
    error_message = ?
WHERE id = ?
RETURNING *;

-- name: RecordWebhookAttempt :one
-- Record a failed delivery attempt (5xx or network error) - stays pending for retry.
UPDATE webhooks
SET attempts = attempts + 1,
    last_attempt_at = datetime('now'),
    error_message = ?
WHERE id = ?
RETURNING *;

-- name: GetPendingWebhooks :many
-- Get webhooks ready for delivery with backoff timing and in-order per endpoint.
-- Only returns the oldest pending webhook per endpoint that has passed its backoff delay.
SELECT w.*, e.destination_url, e.provider_type
FROM webhooks w
JOIN endpoints e ON w.endpoint_id = e.id
WHERE w.status = 'pending'
  AND e.muted = 0
  -- Respect backoff: either never attempted, or backoff delay has passed
  AND (
    w.last_attempt_at IS NULL
    OR datetime(w.last_attempt_at, '+' || MIN(1 << w.attempts, 3600) || ' seconds') <= datetime('now')
  )
  -- In-order delivery: only the oldest pending webhook per endpoint
  AND w.received_at = (
    SELECT MIN(w2.received_at)
    FROM webhooks w2
    WHERE w2.endpoint_id = w.endpoint_id
      AND w2.status = 'pending'
  )
ORDER BY w.received_at ASC
LIMIT ?;


-- name: MarkDeadLetter :execrows
-- Mark pending webhooks as dead_letter after 7 days.
UPDATE webhooks
SET status = 'dead_letter'
WHERE status = 'pending'
  AND received_at < datetime('now', '-7 days');

-- name: GetDeadLetterWebhooks :many
-- Get recently dead-lettered webhooks for notification.
SELECT w.*, e.name as endpoint_name, e.destination_url, e.provider_type
FROM webhooks w
JOIN endpoints e ON w.endpoint_id = e.id
WHERE w.status = 'dead_letter'
ORDER BY w.received_at DESC
LIMIT ?;

-- name: DeleteDeliveredWebhooks :execrows
-- Delete delivered webhooks older than 7 days.
DELETE FROM webhooks
WHERE status = 'delivered'
  AND delivered_at < datetime('now', '-7 days');

-- name: DeleteFailedWebhooks :execrows
-- Delete failed webhooks older than 7 days from last attempt.
DELETE FROM webhooks
WHERE status = 'failed'
  AND last_attempt_at < datetime('now', '-7 days');

-- name: DeleteDeadLetterWebhooks :execrows
-- Delete dead letter webhooks older than 14 days from receipt.
DELETE FROM webhooks
WHERE status = 'dead_letter'
  AND received_at < datetime('now', '-14 days');

-- name: GetQueueStats :one
SELECT
    SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) AS pending_count,
    SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) AS failed_count,
    SUM(CASE WHEN status = 'dead_letter' THEN 1 ELSE 0 END) AS dead_letter_count
FROM webhooks;

-- name: ResetWebhookForReplay :one
UPDATE webhooks
SET status = 'pending',
    attempts = 0,
    last_attempt_at = NULL,
    delivered_at = NULL,
    error_message = NULL,
    notification_sent = 0
WHERE id = ?
RETURNING *;

-- name: GetWebhookWithEndpoint :one
-- Get webhook with endpoint info for notifications.
SELECT w.*, e.name as endpoint_name, e.destination_url as endpoint_destination_url
FROM webhooks w
JOIN endpoints e ON w.endpoint_id = e.id
WHERE w.id = ?;

-- name: MarkNotificationSent :exec
-- Mark notification as sent to prevent spam.
UPDATE webhooks
SET notification_sent = 1
WHERE id = ?;

-- name: GetUnnotifiedDeadLetters :many
-- Get dead letter webhooks that haven't been notified yet.
SELECT w.*, e.name as endpoint_name, e.destination_url as endpoint_destination_url
FROM webhooks w
JOIN endpoints e ON w.endpoint_id = e.id
WHERE w.status = 'dead_letter'
  AND w.notification_sent = 0
ORDER BY w.received_at DESC
LIMIT ?;
