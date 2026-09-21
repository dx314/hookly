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
    -- LEGACY: superseded by the destinations table. Kept (no table rebuild) and
    -- mirrored to the primary destination's URL so older binaries still work.
    destination_url TEXT NOT NULL,
    muted INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_endpoints_user_id ON endpoints(user_id);
CREATE INDEX IF NOT EXISTS idx_endpoints_user_created ON endpoints(user_id, created_at DESC);

-- An endpoint fans out to 1..N destinations. The lowest position is the primary
-- destination (the one legacy clients see as destination_url).
CREATE TABLE IF NOT EXISTS destinations (
    id TEXT PRIMARY KEY,
    endpoint_id TEXT NOT NULL,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    position INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (endpoint_id) REFERENCES endpoints(id) ON DELETE CASCADE,
    UNIQUE (endpoint_id, name)
);

CREATE INDEX IF NOT EXISTS idx_destinations_endpoint_id ON destinations(endpoint_id, position);

-- status, attempts, last_attempt_at, delivered_at and error_message are a rollup
-- of the webhook's deliveries (see db.Rollup); delivery state lives in deliveries.
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

-- One row per (webhook, destination). seq gives arrival order per destination.
CREATE TABLE IF NOT EXISTS deliveries (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    id TEXT NOT NULL UNIQUE,
    webhook_id TEXT NOT NULL,
    destination_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed', 'dead_letter')),
    attempts INTEGER NOT NULL DEFAULT 0,
    last_attempt_at TEXT,
    delivered_at TEXT,
    error_message TEXT,
    notification_sent INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (webhook_id) REFERENCES webhooks(id) ON DELETE CASCADE,
    FOREIGN KEY (destination_id) REFERENCES destinations(id) ON DELETE CASCADE,
    UNIQUE (webhook_id, destination_id)
);

CREATE INDEX IF NOT EXISTS idx_deliveries_webhook_id ON deliveries(webhook_id);
CREATE INDEX IF NOT EXISTS idx_deliveries_destination_status ON deliveries(destination_id, status, seq);


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
