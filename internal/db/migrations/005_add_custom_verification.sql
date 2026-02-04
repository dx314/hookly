-- +goose Up
-- Add custom provider type and verification config for custom header verification.
-- SQLite doesn't support ALTER COLUMN, so we recreate the table.

CREATE TABLE endpoints_new (
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

-- Migrate existing data
INSERT INTO endpoints_new (id, user_id, name, provider_type, signature_secret_encrypted, destination_url, muted, created_at, updated_at)
SELECT id, user_id, name, provider_type, signature_secret_encrypted, destination_url, muted, created_at, updated_at
FROM endpoints;

DROP TABLE endpoints;
ALTER TABLE endpoints_new RENAME TO endpoints;

-- Recreate indexes
CREATE INDEX idx_endpoints_user_id ON endpoints(user_id);
CREATE INDEX idx_endpoints_user_created ON endpoints(user_id, created_at DESC);

-- +goose Down
-- Remove verification_config_encrypted column and custom provider type

CREATE TABLE endpoints_new (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    provider_type TEXT NOT NULL CHECK (provider_type IN ('stripe', 'github', 'telegram', 'generic')),
    signature_secret_encrypted BLOB,
    destination_url TEXT NOT NULL,
    muted INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Migrate existing data (exclude custom endpoints as they won't be valid)
INSERT INTO endpoints_new (id, user_id, name, provider_type, signature_secret_encrypted, destination_url, muted, created_at, updated_at)
SELECT id, user_id, name, provider_type, signature_secret_encrypted, destination_url, muted, created_at, updated_at
FROM endpoints
WHERE provider_type != 'custom';

DROP TABLE endpoints;
ALTER TABLE endpoints_new RENAME TO endpoints;

-- Recreate indexes
CREATE INDEX idx_endpoints_user_id ON endpoints(user_id);
CREATE INDEX idx_endpoints_user_created ON endpoints(user_id, created_at DESC);
