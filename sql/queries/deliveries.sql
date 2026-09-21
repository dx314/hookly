-- name: CreateDelivery :one
INSERT INTO deliveries (id, webhook_id, destination_id, status, attempts, created_at)
VALUES (?, ?, ?, 'pending', 0, datetime('now'))
RETURNING *;

-- name: GetDelivery :one
SELECT * FROM deliveries WHERE id = ?;

-- name: GetDeliveryByWebhookAndDestination :one
SELECT * FROM deliveries WHERE webhook_id = ? AND destination_id = ?;

-- name: ListDeliveriesByWebhook :many
-- Deliveries of a webhook with destination info, primary destination first.
SELECT d.*, dest.name AS destination_name, dest.url AS destination_url, dest.enabled AS destination_enabled
FROM deliveries d
JOIN destinations dest ON d.destination_id = dest.id
WHERE d.webhook_id = ?
ORDER BY dest.position ASC, dest.created_at ASC, dest.id ASC;

-- name: ListPendingDeliveriesByWebhook :many
SELECT d.*
FROM deliveries d
JOIN destinations dest ON d.destination_id = dest.id
WHERE d.webhook_id = ? AND d.status = 'pending'
ORDER BY dest.position ASC, dest.created_at ASC, dest.id ASC;

-- name: ListWebhookIDsByDestination :many
-- Webhooks that have a delivery for this destination (to re-derive their status).
SELECT webhook_id FROM deliveries WHERE destination_id = ?;

-- name: MarkDeliveryDelivered :one
UPDATE deliveries
SET status = 'delivered',
    attempts = attempts + 1,
    last_attempt_at = datetime('now'),
    delivered_at = datetime('now'),
    error_message = NULL
WHERE id = ? AND status = 'pending'
RETURNING *;

-- name: MarkDeliveryFailed :one
-- Permanent failure (4xx response) - no retry.
UPDATE deliveries
SET status = 'failed',
    attempts = attempts + 1,
    last_attempt_at = datetime('now'),
    error_message = ?
WHERE id = ? AND status = 'pending'
RETURNING *;

-- name: RecordDeliveryAttempt :one
-- Transient failure (5xx or network error) - stays pending for retry after backoff.
UPDATE deliveries
SET attempts = attempts + 1,
    last_attempt_at = datetime('now'),
    error_message = ?
WHERE id = ? AND status = 'pending'
RETURNING *;

-- name: ResetDeliveryForReplay :one
UPDATE deliveries
SET status = 'pending',
    attempts = 0,
    last_attempt_at = NULL,
    delivered_at = NULL,
    error_message = NULL,
    notification_sent = 0
WHERE id = ?
RETURNING *;

-- name: GetDispatchableDeliveries :many
-- Deliveries ready to be sent, respecting backoff and in-order delivery per
-- (endpoint, destination): only the oldest pending delivery of each destination
-- is returned, so one failing destination never holds back another.
SELECT d.id AS delivery_id, d.seq, d.attempts, d.destination_id,
       dest.name AS destination_name, dest.url AS destination_url,
       w.id AS webhook_id, w.endpoint_id, w.received_at, w.headers, w.payload
FROM deliveries d
JOIN destinations dest ON d.destination_id = dest.id
JOIN webhooks w ON d.webhook_id = w.id
JOIN endpoints e ON w.endpoint_id = e.id
WHERE d.status = 'pending'
  AND e.muted = 0
  AND dest.enabled = 1
  -- Respect backoff: either never attempted, or backoff delay has passed (1s -> 1h)
  AND (
    d.last_attempt_at IS NULL
    OR datetime(d.last_attempt_at, '+' || MIN(1 << MIN(d.attempts, 12), 3600) || ' seconds') <= datetime('now')
  )
  -- In-order delivery: nothing older is still pending for this destination
  AND NOT EXISTS (
    SELECT 1 FROM deliveries d2
    WHERE d2.destination_id = d.destination_id
      AND d2.status = 'pending'
      AND d2.seq < d.seq
  )
ORDER BY d.seq ASC
LIMIT ?;

-- name: ListExpiredPendingDeliveries :many
-- Pending deliveries whose webhook is older than the 7 day delivery window.
SELECT d.id, d.webhook_id
FROM deliveries d
JOIN webhooks w ON d.webhook_id = w.id
WHERE d.status = 'pending'
  AND w.received_at < datetime('now', '-7 days');

-- name: MarkDeliveryDeadLetter :exec
UPDATE deliveries
SET status = 'dead_letter'
WHERE id = ? AND status = 'pending';

-- name: GetUnnotifiedDeadLetterDeliveries :many
-- Dead-lettered deliveries that haven't been notified yet.
SELECT d.id AS delivery_id, d.attempts, d.error_message,
       w.id AS webhook_id, w.endpoint_id, w.received_at,
       e.user_id, e.name AS endpoint_name, dest.name AS destination_name, dest.url AS destination_url
FROM deliveries d
JOIN destinations dest ON d.destination_id = dest.id
JOIN webhooks w ON d.webhook_id = w.id
JOIN endpoints e ON w.endpoint_id = e.id
WHERE d.status = 'dead_letter'
  AND d.notification_sent = 0
ORDER BY d.seq DESC
LIMIT ?;

-- name: GetDeliveryForNotification :one
SELECT d.id AS delivery_id, d.attempts, d.error_message, d.notification_sent,
       w.id AS webhook_id, w.endpoint_id, w.received_at,
       e.user_id, e.name AS endpoint_name, dest.name AS destination_name, dest.url AS destination_url
FROM deliveries d
JOIN destinations dest ON d.destination_id = dest.id
JOIN webhooks w ON d.webhook_id = w.id
JOIN endpoints e ON w.endpoint_id = e.id
WHERE d.id = ?;

-- name: MarkDeliveryNotificationSent :exec
-- Mark notification as sent to prevent spam.
UPDATE deliveries
SET notification_sent = 1
WHERE id = ?;

-- name: GetDestinationDeliveryStats :many
-- Delivery counts per destination of an endpoint.
SELECT d.destination_id,
       CAST(SUM(CASE WHEN d.status = 'pending' THEN 1 ELSE 0 END) AS INTEGER) AS pending_count,
       CAST(SUM(CASE WHEN d.status = 'delivered' THEN 1 ELSE 0 END) AS INTEGER) AS delivered_count,
       CAST(SUM(CASE WHEN d.status = 'failed' THEN 1 ELSE 0 END) AS INTEGER) AS failed_count,
       CAST(SUM(CASE WHEN d.status = 'dead_letter' THEN 1 ELSE 0 END) AS INTEGER) AS dead_letter_count,
       CAST(COALESCE(MAX(d.delivered_at), '') AS TEXT) AS last_delivered_at
FROM deliveries d
JOIN destinations dest ON d.destination_id = dest.id
WHERE dest.endpoint_id = ?
GROUP BY d.destination_id;

-- name: GetDestinationLastError :one
-- Most recent error of a destination's undelivered deliveries.
SELECT CAST(COALESCE(error_message, '') AS TEXT) AS error_message
FROM deliveries
WHERE destination_id = ? AND status != 'delivered' AND error_message IS NOT NULL
ORDER BY last_attempt_at DESC, seq DESC
LIMIT 1;
