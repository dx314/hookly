-- +goose Up
-- Add user_settings table for per-user preferences and cached GitHub profile data.

CREATE TABLE IF NOT EXISTS user_settings (
    user_id TEXT PRIMARY KEY,
    username TEXT NOT NULL,

    -- GitHub profile (cached from OAuth)
    github_name TEXT,
    github_email TEXT,
    github_profile_url TEXT,
    avatar_url TEXT,

    -- Telegram (token encrypted like endpoint secrets)
    telegram_bot_token_encrypted BLOB,
    telegram_chat_id TEXT,
    telegram_enabled INTEGER NOT NULL DEFAULT 0,

    -- UI preferences
    theme_preference TEXT NOT NULL DEFAULT 'system'
        CHECK (theme_preference IN ('system', 'light', 'dark', 'placid-blue-light', 'placid-blue-dark')),

    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    last_login_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_user_settings_username ON user_settings(username);

-- +goose Down
DROP TABLE IF EXISTS user_settings;
