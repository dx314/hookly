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

-- name: GetWebhookByID :one
-- System query: no user filter (used when re-deriving a webhook's status)
SELECT * FROM webhooks WHERE id = ?;

-- name: UpdateWebhookRollup :exec
-- System query: store the status derived from the webhook's deliveries (see db.Rollup).
UPDATE webhooks
SET status = ?,
    attempts = ?,
    last_attempt_at = ?,
    delivered_at = ?,
    error_message = ?
WHERE id = ?;
