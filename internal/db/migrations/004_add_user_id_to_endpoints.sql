-- +goose Up
-- Add user_id column to endpoints table for multi-tenancy.
-- SQLite doesn't support ALTER COLUMN, so we recreate the table.

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

-- Migrate existing data with placeholder user 'SYSTEM' (admin can reassign later)
INSERT INTO endpoints_new (id, user_id, name, provider_type, signature_secret_encrypted, destination_url, muted, created_at, updated_at)
SELECT id, 'SYSTEM', name, provider_type, signature_secret_encrypted, destination_url, muted, created_at, updated_at
FROM endpoints;

DROP TABLE endpoints;
ALTER TABLE endpoints_new RENAME TO endpoints;

-- Create indexes for efficient user-scoped queries
CREATE INDEX idx_endpoints_user_id ON endpoints(user_id);
CREATE INDEX idx_endpoints_user_created ON endpoints(user_id, created_at DESC);

-- +goose Down
-- Remove user_id column by recreating table

CREATE TABLE endpoints_new (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    provider_type TEXT NOT NULL CHECK (provider_type IN ('stripe', 'github', 'telegram', 'generic')),
    signature_secret_encrypted BLOB,
    destination_url TEXT NOT NULL,
    muted INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO endpoints_new (id, name, provider_type, signature_secret_encrypted, destination_url, muted, created_at, updated_at)
SELECT id, name, provider_type, signature_secret_encrypted, destination_url, muted, created_at, updated_at
FROM endpoints;

DROP TABLE endpoints;
ALTER TABLE endpoints_new RENAME TO endpoints;
