-- name: CreateAPIToken :one
INSERT INTO api_tokens (id, user_id, username, token_hash, name)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetAPITokenByHash :one
SELECT * FROM api_tokens
WHERE token_hash = ?
  AND revoked = 0;

-- name: GetAPITokensByUser :many
SELECT * FROM api_tokens
WHERE user_id = ?
ORDER BY created_at DESC;

-- name: UpdateAPITokenLastUsed :exec
UPDATE api_tokens
SET last_used_at = datetime('now')
WHERE id = ?;

-- name: RevokeAPIToken :exec
UPDATE api_tokens
SET revoked = 1
WHERE id = ?;

-- name: RevokeAllUserAPITokens :exec
UPDATE api_tokens
SET revoked = 1
WHERE user_id = ?;

-- name: DeleteRevokedAPITokens :execrows
DELETE FROM api_tokens
WHERE revoked = 1
  AND (last_used_at IS NULL OR last_used_at < datetime('now', '-30 days'));
