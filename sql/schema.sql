-- Hookly Database Schema
-- This file is used by sqlc for code generation.
-- Actual migrations are in internal/db/migrations/

CREATE TABLE IF NOT EXISTS endpoints (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    provider_type TEXT NOT NULL CHECK (provider_type IN ('stripe', 'github', 'telegram', 'generic', 'custom')),
    signature_secret_encrypted BLOB,
    verification_config_encrypted BLOB,
    destination_url TEXT NOT NULL,
    muted INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_endpoints_user_id ON endpoints(user_id);
CREATE INDEX IF NOT EXISTS idx_endpoints_user_created ON endpoints(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS webhooks (
    id TEXT PRIMARY KEY,
    endpoint_id TEXT NOT NULL,
    received_at TEXT NOT NULL DEFAULT (datetime('now')),
    headers TEXT NOT NULL,  -- JSON encoded
    payload BLOB NOT NULL,
    signature_valid INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed', 'dead_letter')),
    attempts INTEGER NOT NULL DEFAULT 0,
    last_attempt_at TEXT,
    delivered_at TEXT,
    error_message TEXT,
    notification_sent INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (endpoint_id) REFERENCES endpoints(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_webhooks_endpoint_id ON webhooks(endpoint_id);
CREATE INDEX IF NOT EXISTS idx_webhooks_status ON webhooks(status);
CREATE INDEX IF NOT EXISTS idx_webhooks_received_at ON webhooks(received_at);
CREATE INDEX IF NOT EXISTS idx_webhooks_status_received ON webhooks(status, received_at);

CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    username TEXT NOT NULL,
    avatar_url TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

CREATE TABLE IF NOT EXISTS api_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    username TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    last_used_at TEXT,
    revoked INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_api_tokens_hash ON api_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_api_tokens_user ON api_tokens(user_id);

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
