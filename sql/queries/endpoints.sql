-- name: CreateEndpoint :one
INSERT INTO endpoints (id, name, provider_type, signature_secret_encrypted, destination_url, muted, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, 0, datetime('now'), datetime('now'))
RETURNING *;

-- name: GetEndpoint :one
SELECT * FROM endpoints WHERE id = ?;

-- name: ListEndpoints :many
SELECT * FROM endpoints ORDER BY created_at DESC LIMIT ? OFFSET ?;

-- name: CountEndpoints :one
SELECT COUNT(*) FROM endpoints;

-- name: UpdateEndpoint :one
UPDATE endpoints
SET name = COALESCE(sqlc.narg('name'), name),
    signature_secret_encrypted = COALESCE(sqlc.narg('signature_secret_encrypted'), signature_secret_encrypted),
    destination_url = COALESCE(sqlc.narg('destination_url'), destination_url),
    muted = COALESCE(sqlc.narg('muted'), muted),
    updated_at = datetime('now')
WHERE id = ?
RETURNING *;

-- name: DeleteEndpoint :exec
DELETE FROM endpoints WHERE id = ?;

-- name: GetEndpointByID :one
SELECT id, name, provider_type, signature_secret_encrypted, destination_url, muted
FROM endpoints
WHERE id = ?;
