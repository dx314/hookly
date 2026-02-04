# Work Order 04: Home Hub

**Swimlane:** Backend
**Status:** DONE
**Dependencies:** 00-foundation, 03-relay

---

## Objective

Implement home-hub service that receives webhooks and forwards to destinations.

---

## Tasks

### Config
- [x] Create `internal/config/home.go`:
  ```go
  type HomeConfig struct {
    EdgeURL       string  // https://hooks.dx314.com
    HomeHubSecret string  // Pre-shared secret
    HubID         string  // Identifier for this hub
  }
  ```
- [x] Load from environment variables

### Main Entry Point
- [x] Create `cmd/home-hub/main.go`:
  - Load config
  - Initialize relay client
  - Connect to edge
  - Handle webhooks
  - Graceful shutdown

### Relay Client
- [x] Create `internal/relay/client.go`:
  - Connect to edge RelayService
  - Authenticate with HMAC
  - Receive webhook stream
  - Send ACKs
  - Auto-reconnect on disconnect (exponential backoff)

### Webhook Forwarder
- [x] Create `internal/webhook/forwarder.go`:
  - Receive WebhookEnvelope
  - POST to destination_url
  - Include original headers (filtered)
  - Include original payload
  - Return status code and error

### Header Filtering
- [x] Filter headers for forwarding:
  - Remove: `Host`, `Content-Length` (recalculated)
  - Keep: `Content-Type`, `X-*` headers, webhook-specific headers
  - Add: `X-Hookly-Webhook-Id`, `X-Hookly-Attempt`

### Delivery Logic
- [x] On 2xx response: ACK success
- [x] On 4xx response: ACK failure (permanent)
- [x] On 5xx response: ACK failure (will retry)
- [x] On network error: ACK failure (will retry)

### Connection Management
- [x] Initial connection attempt on startup
- [x] Reconnect on disconnect:
  - Backoff: 1s, 2s, 4s, 8s... max 60s
  - Log reconnection attempts
- [x] Send heartbeat every 30 seconds

### Logging
- [x] Log webhook received
- [x] Log delivery attempt (destination, attempt #)
- [x] Log delivery result (success/failure, status code)
- [x] Log connection state changes

---

## Acceptance Criteria

- [x] Home-hub connects to edge on startup
- [x] Authenticates successfully with valid secret
- [x] Receives webhooks from edge
- [x] Forwards to destination URL
- [x] ACKs sent back to edge
- [x] 2xx → success ACK
- [x] 4xx → permanent failure ACK
- [x] 5xx → transient failure ACK
- [x] Auto-reconnects on disconnect
- [x] Heartbeat keeps connection alive
- [x] Graceful shutdown (finish in-flight deliveries)

---

## Notes

- HTTP client timeout: 30 seconds
- Don't follow redirects (forward as-is)
- Respect `Content-Type` from original webhook
