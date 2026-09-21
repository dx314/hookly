-- +goose Up
-- Multi-destination fan-out: an endpoint delivers to 1..N destinations and
-- delivery state is tracked per (webhook, destination).
--
-- Additive and in place: no existing table is rebuilt or altered. The legacy
-- endpoints.destination_url column stays and mirrors the primary destination.
-- Back-fill: every endpoint gets one destination ("default") from its
-- destination_url, and every webhook gets one delivery row carrying its own
-- status/attempts/timestamps/error, so history and in-flight retries survive.
-- Back-filled IDs are random hex (nanoid is not available in SQL).

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

INSERT INTO destinations (id, endpoint_id, name, url, enabled, position, created_at, updated_at)
SELECT 'dst_' || lower(hex(randomblob(12))), e.id, 'default', e.destination_url, 1, 0, e.created_at, e.updated_at
FROM endpoints e
WHERE NOT EXISTS (SELECT 1 FROM destinations d WHERE d.endpoint_id = e.id);

-- seq follows arrival order so per-destination ordering is preserved
INSERT INTO deliveries (id, webhook_id, destination_id, status, attempts, last_attempt_at,
                        delivered_at, error_message, notification_sent, created_at)
SELECT 'dl_' || lower(hex(randomblob(12))), w.id,
       (SELECT d.id FROM destinations d WHERE d.endpoint_id = w.endpoint_id
        ORDER BY d.position ASC, d.created_at ASC, d.id ASC LIMIT 1),
       w.status, w.attempts, w.last_attempt_at, w.delivered_at, w.error_message,
       w.notification_sent, w.received_at
FROM webhooks w
WHERE NOT EXISTS (SELECT 1 FROM deliveries dl WHERE dl.webhook_id = w.id)
ORDER BY w.received_at ASC, w.rowid ASC;

-- +goose Down
-- webhooks.status/attempts/... hold the derived rollup and endpoints.destination_url
-- mirrors the primary destination, so the single-destination schema is intact.
DROP TABLE IF EXISTS deliveries;
DROP TABLE IF EXISTS destinations;
