# Work Order 02: Edge Core

**Swimlane:** Backend
**Status:** DONE
**Dependencies:** 00-foundation, 01-database

---

## Objective

Implement webhook ingestion endpoint with signature verification.

---

## Tasks

### Config
- [x] Create `internal/config/config.go`:
  ```go
  type Config struct {
    DatabasePath      string
    EncryptionKey     []byte
    Port              int
    BaseURL           string
    HomeHubSecret     string
    GitHubClientID    string
    GitHubClientSecret string
    GitHubOrg         string
    GitHubAllowedUsers []string
    TelegramBotToken  string
    TelegramChatID    string
  }
  ```
- [x] Load from environment variables
- [x] Validate required fields

### HTTP Server
- [x] Create `internal/server/server.go`:
  - chi router setup
  - Middleware (logging, CORS)
  - Graceful shutdown
- [x] Create `cmd/edge-gateway/main.go`:
  - Load config
  - Initialize database
  - Run migrations
  - Start server

### Webhook Ingestion
- [x] Create `internal/webhook/handler.go`:
  - `POST /h/{endpoint-id}` handler
  - Look up endpoint by ID (return 404 if not found)
  - Read body (limit 100MB)
  - Extract headers
  - Verify signature (per provider type)
  - Store webhook with signature_valid flag
  - Return 200 immediately (async processing)

### Signature Verification
- [x] Create `internal/webhook/verify.go`:
  - `Verifier` interface
  - `StripeVerifier`: HMAC-SHA256 with `Stripe-Signature` header
  - `GitHubVerifier`: HMAC-SHA256 with `X-Hub-Signature-256` header
  - `TelegramVerifier`: Validate `X-Telegram-Bot-Api-Secret-Token` header
  - `GenericVerifier`: HMAC-SHA256 with `X-Webhook-Signature` header
- [x] Factory function: `NewVerifier(providerType) Verifier`

### Stripe Signature Format
```
Stripe-Signature: t=1492774577,v1=5257a869...
```
- Parse timestamp and signature from header
- Compute: HMAC-SHA256(timestamp + "." + payload, secret)
- Compare with v1 value

### GitHub Signature Format
```
X-Hub-Signature-256: sha256=d57c68ca...
```
- Compute: HMAC-SHA256(payload, secret)
- Compare with header value (hex encoded)

### Telegram Verification
```
X-Telegram-Bot-Api-Secret-Token: <secret>
```
- Simple string comparison with configured secret

### Generic Signature Format
```
X-Webhook-Signature: sha256=...
```
- Same as GitHub format

---

## Acceptance Criteria

- [x] `POST /h/{valid-id}` returns 200 and stores webhook
- [x] `POST /h/{invalid-id}` returns 404
- [x] Stripe webhook with valid signature → `signature_valid=true`
- [x] Stripe webhook with invalid signature → `signature_valid=false`, still stored
- [x] GitHub webhook verification works
- [x] Telegram webhook verification works
- [x] Generic webhook verification works
- [x] Payload up to 100MB accepted
- [x] Headers stored as JSON

---

## Notes

- Don't reject invalid signatures - store with `signature_valid=false`
- Log verification failures for debugging
- Use constant-time comparison for signatures
- Added `internal/server/middleware.go` for logging and CORS
- Added `internal/webhook/verify_test.go` with comprehensive tests
