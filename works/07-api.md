# Work Order 07: API Layer

**Swimlane:** Backend
**Status:** DONE
**Dependencies:** 01-database, 06-auth

---

## Objective

Implement ConnectRPC API for endpoints, webhooks, and system status.

---

## Tasks

### EdgeService Implementation
- [x] Create `internal/service/edge/service.go`:
  - Implement all EdgeService RPCs

### Endpoint CRUD
- [x] `CreateEndpoint(request) → Endpoint`:
  - Generate nanoid for ID
  - Encrypt signature_secret
  - Store in database
  - Return created endpoint (with generated webhook URL)
- [x] `GetEndpoint(id) → Endpoint`:
  - Fetch from database
  - Return 404 if not found
  - Note: signature_secret not exposed in responses (as per proto spec)
- [x] `ListEndpoints() → []Endpoint`:
  - Fetch all endpoints with pagination
- [x] `UpdateEndpoint(id, updates) → Endpoint`:
  - Partial update support
  - Re-encrypt secret if changed
- [x] `DeleteEndpoint(id)`:
  - Cascade delete webhooks (via FK)
  - Return 404 if not found
- [x] `MuteEndpoint(id, muted) → Endpoint`:
  - Toggle muted flag via UpdateEndpoint
  - Muted endpoints don't forward webhooks

### Webhook Operations
- [x] `ListWebhooks(filters) → []Webhook`:
  - Filter by endpoint_id
  - Filter by status
  - Pagination (limit, offset)
- [x] `GetWebhook(id) → Webhook`:
  - Include full payload
  - Include headers
- [x] `ReplayWebhook(id)`:
  - Reset status to pending
  - Reset attempts to 0
  - Clear error_message
  - Will be picked up by relay

### System Status
- [x] `GetStatus() → Status`:
  - pending_count
  - failed_count
  - dead_letter_count
  - home_hub_connected
  - last_home_hub_heartbeat

### Settings
- [x] `GetSettings() → Settings`:
  - Return config (redact secrets)
  - Show base URL, auth/notification status

### Webhook URL Generation
- [x] Generate URL: `{BASE_URL}/h/{endpoint_id}`
- [x] Include in endpoint responses

### ConnectRPC Registration
- [x] Register EdgeService handler in main.go
- [x] Apply auth interceptor when auth is enabled

### Interceptor
- [x] Create `internal/server/interceptor.go`:
  - Extract session from Cookie header
  - Validate via SessionManager
  - Pass session to context

---

## Acceptance Criteria

- [x] All CRUD operations work via ConnectRPC
- [x] Endpoints return generated webhook URL
- [x] Webhooks filterable by endpoint and status
- [x] Replay resets webhook for re-delivery
- [x] Status shows queue depth and connection state
- [x] All operations require authentication (when auth configured)
- [x] Proper error codes (NOT_FOUND, INVALID_ARGUMENT, UNAUTHENTICATED)

---

## Files Created/Modified

- `internal/service/edge/service.go` - EdgeService implementation
- `internal/server/interceptor.go` - Auth interceptor for ConnectRPC
- `internal/auth/middleware.go` - Added ContextWithSession helper
- `cmd/edge-gateway/main.go` - Registered EdgeService handler

---

## Notes

- Use ConnectRPC error codes correctly:
  - `connect.CodeNotFound` for 404
  - `connect.CodeInvalidArgument` for validation errors
  - `connect.CodeUnauthenticated` for auth failures
- When auth is not configured (dev mode), EdgeService runs without auth
- Auth interceptor uses session cookies (not JWT) to match existing auth system
