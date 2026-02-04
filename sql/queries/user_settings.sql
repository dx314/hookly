-- name: GetUserSettings :one
SELECT * FROM user_settings WHERE user_id = ?;

-- name: GetUserSettingsByUsername :one
SELECT * FROM user_settings WHERE username = ?;

-- name: UpsertUserSettings :one
INSERT INTO user_settings (
    user_id,
    username,
    github_name,
    github_email,
    github_profile_url,
    avatar_url,
    last_login_at
)
VALUES (?, ?, ?, ?, ?, ?, datetime('now'))
ON CONFLICT(user_id) DO UPDATE SET
    username = excluded.username,
    github_name = excluded.github_name,
    github_email = excluded.github_email,
    github_profile_url = excluded.github_profile_url,
    avatar_url = excluded.avatar_url,
    last_login_at = datetime('now'),
    updated_at = datetime('now')
RETURNING *;

-- name: UpdateUserTelegramSettings :one
UPDATE user_settings
SET telegram_bot_token_encrypted = ?,
    telegram_chat_id = ?,
    telegram_enabled = ?,
    updated_at = datetime('now')
WHERE user_id = ?
RETURNING *;

-- name: UpdateUserTheme :one
UPDATE user_settings
SET theme_preference = ?,
    updated_at = datetime('now')
WHERE user_id = ?
RETURNING *;

-- name: GetEndpointOwnerTelegramConfig :one
-- Get the endpoint owner's Telegram configuration for sending notifications
SELECT
    us.user_id,
    us.telegram_bot_token_encrypted,
    us.telegram_chat_id,
    us.telegram_enabled
FROM endpoints e
JOIN user_settings us ON e.user_id = us.user_id
WHERE e.id = ?;

-- name: CountUsers :one
SELECT COUNT(*) FROM user_settings;

-- name: CountAllEndpoints :one
SELECT COUNT(*) FROM endpoints;
