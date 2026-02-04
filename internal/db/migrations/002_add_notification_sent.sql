-- +goose Up
-- Add notification_sent column to track Telegram notifications

-- Check if column exists before adding (SQLite doesn't support IF NOT EXISTS for columns)
-- This is handled by goose's migration tracking - if this ran, the column exists

ALTER TABLE webhooks ADD COLUMN notification_sent INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite doesn't support DROP COLUMN before 3.35.0, so we recreate the table

CREATE TABLE webhooks_new (
    id TEXT PRIMARY KEY,
    endpoint_id TEXT NOT NULL,
    received_at TEXT NOT NULL DEFAULT (datetime('now')),
    headers TEXT NOT NULL,
    payload BLOB NOT NULL,
    signature_valid INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed', 'dead_letter')),
    attempts INTEGER NOT NULL DEFAULT 0,
    last_attempt_at TEXT,
    delivered_at TEXT,
    error_message TEXT,
    FOREIGN KEY (endpoint_id) REFERENCES endpoints(id) ON DELETE CASCADE
);

INSERT INTO webhooks_new SELECT id, endpoint_id, received_at, headers, payload, signature_valid, status, attempts, last_attempt_at, delivered_at, error_message FROM webhooks;

DROP TABLE webhooks;
ALTER TABLE webhooks_new RENAME TO webhooks;

CREATE INDEX idx_webhooks_endpoint_id ON webhooks(endpoint_id);
CREATE INDEX idx_webhooks_status ON webhooks(status);
CREATE INDEX idx_webhooks_received_at ON webhooks(received_at);
CREATE INDEX idx_webhooks_status_received ON webhooks(status, received_at);
