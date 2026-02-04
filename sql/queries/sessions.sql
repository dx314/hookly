-- name: CreateSession :one
INSERT INTO sessions (id, user_id, username, avatar_url, expires_at)
VALUES (?, ?, ?, ?, datetime('now', '+7 days'))
RETURNING *;

-- name: GetSession :one
SELECT * FROM sessions
WHERE id = ?
  AND expires_at > datetime('now');

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = ?;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE expires_at < datetime('now');

-- name: DeleteUserSessions :exec
DELETE FROM sessions WHERE user_id = ?;
