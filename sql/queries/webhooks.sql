-- name: CreateWebhook :one
INSERT INTO webhooks (id, endpoint_id, received_at, headers, payload, signature_valid, status, attempts)
VALUES (?, ?, datetime('now'), ?, ?, ?, 'pending', 0)
RETURNING *;

-- name: GetWebhook :one
-- User-facing query: validates endpoint ownership via JOIN
SELECT w.* FROM webhooks w
JOIN endpoints e ON w.endpoint_id = e.id
WHERE w.id = ? AND e.user_id = ?;

-- name: ListWebhooks :many
-- User-facing query: filters by endpoint ownership
SELECT w.* FROM webhooks w
JOIN endpoints e ON w.endpoint_id = e.id
WHERE e.user_id = sqlc.arg('user_id')
  AND (sqlc.arg('endpoint_id') IS NULL OR w.endpoint_id = sqlc.arg('endpoint_id'))
  AND (sqlc.arg('status') IS NULL OR w.status = sqlc.arg('status'))
ORDER BY w.received_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountWebhooks :one
-- User-facing query: counts webhooks owned by user
SELECT COUNT(*) FROM webhooks w
JOIN endpoints e ON w.endpoint_id = e.id
WHERE e.user_id = sqlc.arg('user_id')
  AND (sqlc.arg('endpoint_id') IS NULL OR w.endpoint_id = sqlc.arg('endpoint_id'))
  AND (sqlc.arg('status') IS NULL OR w.status = sqlc.arg('status'));

-- name: MarkWebhookDelivered :one
-- System query: no user filter (called by background dispatcher)
UPDATE webhooks
SET status = 'delivered',
    attempts = attempts + 1,
    last_attempt_at = datetime('now'),
    delivered_at = datetime('now'),
    error_message = NULL
WHERE id = ?
RETURNING *;

-- name: MarkWebhookFailed :one
-- System query: no user filter (called by background dispatcher)
UPDATE webhooks
SET status = 'failed',
    attempts = attempts + 1,
    last_attempt_at = datetime('now'),
    error_message = ?
WHERE id = ?
RETURNING *;

-- name: RecordWebhookAttempt :one
-- System query: no user filter (called by background dispatcher)
UPDATE webhooks
SET attempts = attempts + 1,
    last_attempt_at = datetime('now'),
    error_message = ?
WHERE id = ?
RETURNING *;

-- name: GetPendingWebhooks :many
-- System query: gets all pending webhooks for dispatch (no user filter)
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
-- System query: marks old pending webhooks as dead_letter (no user filter)
UPDATE webhooks
SET status = 'dead_letter'
WHERE status = 'pending'
  AND received_at < datetime('now', '-7 days');

-- name: GetDeadLetterWebhooks :many
-- System query: gets dead letter webhooks for admin notification (no user filter)
SELECT w.*, e.name as endpoint_name, e.destination_url, e.provider_type
FROM webhooks w
JOIN endpoints e ON w.endpoint_id = e.id
WHERE w.status = 'dead_letter'
ORDER BY w.received_at DESC
LIMIT ?;

-- name: DeleteDeliveredWebhooks :execrows
-- System query: cleanup old delivered webhooks (no user filter)
DELETE FROM webhooks
WHERE status = 'delivered'
  AND delivered_at < datetime('now', '-7 days');

-- name: DeleteFailedWebhooks :execrows
-- System query: cleanup old failed webhooks (no user filter)
DELETE FROM webhooks
WHERE status = 'failed'
  AND last_attempt_at < datetime('now', '-7 days');

-- name: DeleteDeadLetterWebhooks :execrows
-- System query: cleanup old dead letter webhooks (no user filter)
DELETE FROM webhooks
WHERE status = 'dead_letter'
  AND received_at < datetime('now', '-14 days');

-- name: GetQueueStats :one
-- User-facing query: gets queue stats for user's endpoints
SELECT
    SUM(CASE WHEN w.status = 'pending' THEN 1 ELSE 0 END) AS pending_count,
    SUM(CASE WHEN w.status = 'failed' THEN 1 ELSE 0 END) AS failed_count,
    SUM(CASE WHEN w.status = 'dead_letter' THEN 1 ELSE 0 END) AS dead_letter_count
FROM webhooks w
JOIN endpoints e ON w.endpoint_id = e.id
WHERE e.user_id = ?;

-- name: ResetWebhookForReplay :one
-- User-facing query: validates endpoint ownership via subquery
UPDATE webhooks
SET status = 'pending',
    attempts = 0,
    last_attempt_at = NULL,
    delivered_at = NULL,
    error_message = NULL,
    notification_sent = 0
WHERE webhooks.id = ?
  AND webhooks.endpoint_id IN (SELECT e.id FROM endpoints e WHERE e.user_id = ?)
RETURNING *;

-- name: GetWebhookWithEndpoint :one
-- User-facing query: gets webhook with endpoint info, validates ownership
SELECT w.*, e.name as endpoint_name, e.destination_url as endpoint_destination_url
FROM webhooks w
JOIN endpoints e ON w.endpoint_id = e.id
WHERE w.id = ? AND e.user_id = ?;

-- name: GetWebhookWithEndpointByID :one
-- System query: gets webhook with endpoint info for notifications (no user filter)
SELECT w.*, e.name as endpoint_name, e.destination_url as endpoint_destination_url
FROM webhooks w
JOIN endpoints e ON w.endpoint_id = e.id
WHERE w.id = ?;

-- name: MarkNotificationSent :exec
-- System query: marks notification as sent (no user filter)
UPDATE webhooks
SET notification_sent = 1
WHERE id = ?;

-- name: GetUnnotifiedDeadLetters :many
-- System query: gets unnotified dead letters for admin (no user filter)
SELECT w.*, e.name as endpoint_name, e.destination_url as endpoint_destination_url
FROM webhooks w
JOIN endpoints e ON w.endpoint_id = e.id
WHERE w.status = 'dead_letter'
  AND w.notification_sent = 0
ORDER BY w.received_at DESC
LIMIT ?;
