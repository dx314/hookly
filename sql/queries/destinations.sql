-- name: CreateDestination :one
INSERT INTO destinations (id, endpoint_id, name, url, enabled, position, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
RETURNING *;

-- name: GetDestination :one
-- System query: no user filter
SELECT * FROM destinations WHERE id = ?;

-- name: GetDestinationForUser :one
-- User-facing query: validates endpoint ownership via JOIN
SELECT d.* FROM destinations d
JOIN endpoints e ON d.endpoint_id = e.id
WHERE d.id = ? AND e.user_id = ?;

-- name: ListDestinationsByEndpoint :many
-- Ordered so that the first row is the endpoint's primary destination.
SELECT * FROM destinations
WHERE endpoint_id = ?
ORDER BY position ASC, created_at ASC, id ASC;

-- name: ListEnabledDestinationsByEndpoint :many
SELECT * FROM destinations
WHERE endpoint_id = ? AND enabled = 1
ORDER BY position ASC, created_at ASC, id ASC;

-- name: CountDestinationsByEndpoint :one
SELECT COUNT(*) FROM destinations WHERE endpoint_id = ?;

-- name: CountEnabledDestinationsByEndpoint :one
SELECT COUNT(*) FROM destinations WHERE endpoint_id = ? AND enabled = 1;

-- name: NextDestinationPosition :one
SELECT CAST(COALESCE(MAX(position), -1) + 1 AS INTEGER) FROM destinations WHERE endpoint_id = ?;

-- name: UpdateDestination :one
UPDATE destinations
SET name = COALESCE(sqlc.narg('name'), name),
    url = COALESCE(sqlc.narg('url'), url),
    enabled = COALESCE(sqlc.narg('enabled'), enabled),
    updated_at = datetime('now')
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeleteDestination :exec
-- Deliveries for the destination are removed by ON DELETE CASCADE.
DELETE FROM destinations WHERE id = ?;
