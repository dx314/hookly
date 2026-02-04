# Work Order 03: Relay Service

**Swimlane:** Backend
**Status:** DONE
**Dependencies:** 00-foundation, 01-database, 02-edge-core

---

## Objective

Implement gRPC streaming service for edge↔home communication.

---

## Tasks

### Proto Definition (relay.proto)
- [x] Define RelayService (completed in 00-foundation):
  - StreamRequest/StreamResponse with oneof for message types
  - ConnectRequest/ConnectResponse for authentication
  - WebhookEnvelope for delivery
  - DeliveryAck for acknowledgments
  - Heartbeat for connection health

### Edge Relay Handler
- [x] Create `internal/relay/handler.go`:
  - Implement `RelayServiceHandler`
  - Handle authentication (first message must be ConnectRequest)
  - Validate HMAC: `HMAC-SHA256(hubID:timestamp, secret)`
  - Check timestamp within ±5 minutes
  - Track connected home-hub (singleton for now)
  - Push pending webhooks to connected client
  - Receive ACKs and update webhook status

### Connection Manager
- [x] Create `internal/relay/manager.go`:
  - Track active connection
  - Expose `IsConnected() bool`
  - Expose `Send(webhook) error`
  - Handle reconnection (home-hub initiates)
  - Buffer up to 1000 webhooks

### Webhook Dispatcher
- [x] Create `internal/relay/dispatcher.go`:
  - Watch for new pending webhooks (1 second interval)
  - Push to connected home-hub
  - Handle backpressure (buffer if home offline)

### HMAC Authentication
- [x] Create `internal/relay/auth.go`:
  - `GenerateHMAC(hubID, timestamp, secret) string`
  - `ValidateHMAC(hubID, timestamp, hmac, secret) bool`
  - Constant-time comparison

### Heartbeat
- [x] Implement heartbeat every 30 seconds
- [x] Detect stale connections (no heartbeat for 60s)
- [x] Close stale connections

---

## Acceptance Criteria

- [x] Home-hub can connect with valid HMAC
- [x] Invalid HMAC rejected with error
- [x] Expired timestamp (>5 min) rejected
- [x] Pending webhooks pushed to connected home-hub
- [x] ACKs update webhook status in database
- [x] Connection status exposed for dashboard
- [x] Heartbeat keeps connection alive
- [x] Stale connections cleaned up

---

## Notes

- Use bidirectional streaming for simplicity
- Single home-hub connection (no multi-tenant)
- Webhooks delivered in-order per endpoint
- Added `internal/relay/auth_test.go` with tests
- Integrated relay service into edge-gateway main.go
