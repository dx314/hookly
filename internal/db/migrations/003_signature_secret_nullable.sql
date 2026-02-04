-- +goose Up
-- Make signature_secret_encrypted nullable to allow endpoints without signature verification

-- SQLite doesn't support ALTER COLUMN, so we recreate the table
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

INSERT INTO endpoints_new SELECT * FROM endpoints;

DROP TABLE endpoints;
ALTER TABLE endpoints_new RENAME TO endpoints;

-- +goose Down
-- Revert to NOT NULL (will fail if any NULL values exist)

CREATE TABLE endpoints_new (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    provider_type TEXT NOT NULL CHECK (provider_type IN ('stripe', 'github', 'telegram', 'generic')),
    signature_secret_encrypted BLOB NOT NULL,
    destination_url TEXT NOT NULL,
    muted INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO endpoints_new SELECT * FROM endpoints;

DROP TABLE endpoints;
ALTER TABLE endpoints_new RENAME TO endpoints;
