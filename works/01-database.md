# Work Order 01: Database

**Swimlane:** Backend
**Status:** DONE
**Dependencies:** 00-foundation

---

## Objective

Set up SQLite database with schema, migrations, and query generation.

---

## Tasks

### Schema Design
- [x] Create `sql/schema.sql`:
  ```sql
  CREATE TABLE endpoints (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    provider_type TEXT NOT NULL CHECK (provider_type IN ('stripe', 'github', 'telegram', 'generic')),
    signature_secret_encrypted BLOB NOT NULL,
    destination_url TEXT NOT NULL,
    muted INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
  );

  CREATE TABLE webhooks (
    id TEXT PRIMARY KEY,
    endpoint_id TEXT NOT NULL REFERENCES endpoints(id) ON DELETE CASCADE,
    received_at TEXT NOT NULL DEFAULT (datetime('now')),
    headers TEXT NOT NULL,  -- JSON
    payload BLOB NOT NULL,
    signature_valid INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed', 'dead_letter')),
    attempts INTEGER NOT NULL DEFAULT 0,
    last_attempt_at TEXT,
    delivered_at TEXT,
    error_message TEXT,
    FOREIGN KEY (endpoint_id) REFERENCES endpoints(id)
  );

  CREATE INDEX idx_webhooks_endpoint_id ON webhooks(endpoint_id);
  CREATE INDEX idx_webhooks_status ON webhooks(status);
  CREATE INDEX idx_webhooks_received_at ON webhooks(received_at);
  ```

### sqlc Setup
- [x] Create `sqlc.yaml` config
- [x] Create `sql/queries/endpoints.sql`:
  - CreateEndpoint
  - GetEndpoint
  - ListEndpoints
  - UpdateEndpoint
  - DeleteEndpoint
  - GetEndpointByID (for webhook ingestion lookup)
- [x] Create `sql/queries/webhooks.sql`:
  - CreateWebhook
  - GetWebhook
  - ListWebhooks (with filters)
  - UpdateWebhookStatus
  - GetPendingWebhooks (for relay)
  - GetWebhooksForRetry
  - MarkDeadLetter (webhooks older than 7 days)
  - DeleteOldWebhooks (retention cleanup)
  - GetQueueStats
- [x] Run `sqlc generate`

### Migration System
- [x] Create `internal/db/migrations.go` with embedded SQL
- [x] Implement auto-migration on startup

### Encryption
- [x] Create `internal/crypto/aes.go`:
  - Encrypt(plaintext []byte, key []byte) ([]byte, error)
  - Decrypt(ciphertext []byte, key []byte) ([]byte, error)
  - Uses AES-256-GCM
- [x] Create `internal/db/secrets.go`:
  - EncryptSecret / DecryptSecret helpers

---

## Acceptance Criteria

- [x] SQLite database created on first run
- [x] Schema migrations apply cleanly
- [x] sqlc generates type-safe queries
- [x] Secrets encrypted at rest
- [x] `internal/db` package exposes clean interface

---

## Notes

- Use `database/sql` with `mattn/go-sqlite3` driver
- Enable foreign keys: `PRAGMA foreign_keys = ON`
- Enable WAL mode: `PRAGMA journal_mode = WAL`
- Added `internal/db/connect.go` for connection helper
- Added `internal/db/db_test.go` with tests verifying functionality
