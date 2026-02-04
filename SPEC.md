# Hookly Specification

**Status:** FINAL — Ready for implementation

---

## Problem Statement

Route webhooks from external providers (Stripe, GitHub, Telegram, etc.) to services on a home network with no inbound WAN exposure. Must:
- Accept webhooks at public edge (`hooks.dx314.com`)
- Generate unique URLs per endpoint
- Verify signatures (mark unverified if invalid, don't reject)
- Buffer when home offline, deliver later, in-order per-endpoint
- Provide remote UI to view/replay failed webhooks

---

## Architecture

### Components

```
┌─────────────────────────────────────────────────────────────┐
│                    EDGE (public server)                     │
│  ┌───────────────────────────────┐  ┌───────────────────┐  │
│  │       edge-gateway (Go)       │  │  SQLite           │  │
│  │  • Webhook ingestion          │  │  (webhooks,       │  │
│  │  • ConnectRPC API             │  │   endpoints,      │  │
│  │  • Embedded Svelte UI         │  │   config)         │  │
│  │  • gRPC streaming server      │  │                   │  │
│  └──────────────┬────────────────┘  └───────────────────┘  │
│                 │                                           │
└─────────────────┼───────────────────────────────────────────┘
                  │ gRPC streaming (HMAC-authenticated)
                  │ home-hub initiates outbound connection
                  ▼
┌─────────────────────────────────────────────────────────────┐
│                   HOME (private network)                    │
│  ┌─────────────┐                                            │
│  │  home-hub   │──► Jellyfin, Otto, Home Assistant, etc.   │
│  │   (Go)      │                                            │
│  └─────────────┘                                            │
└─────────────────────────────────────────────────────────────┘
```

### Data Flow

1. Webhook provider POSTs to `hooks.dx314.com/h/{endpoint-id}`
2. edge-gateway verifies signature (or marks unverified)
3. edge-gateway stores webhook in SQLite queue
4. edge-gateway pushes to home-hub via persistent connection
5. home-hub forwards to destination URL on home network
6. home-hub ACKs to edge-gateway
7. edge-gateway marks webhook as delivered

### Connection Model

- **Home → Edge**: Home-hub initiates persistent gRPC streaming connection to edge-gateway
- **Protocol**: ConnectRPC + protobuf (buf v2 codegen stack)
- **Authentication**: Pre-shared secret, HMAC-signed messages with timestamp
- **No VPN required**: All traffic over public internet, app-level security

---

## Services

### edge-gateway (Go) — single binary, single container
- HTTP server for webhook ingestion (`/h/{endpoint-id}`)
- ConnectRPC API for UI and MCP
- gRPC streaming server for home-hub connection
- Signature verification per provider type (Stripe, GitHub, Telegram, Generic)
- SQLite storage for queue, endpoints, config
- GitHub OAuth integration
- Telegram notification sender
- **Embedded SvelteKit UI** (static build served from Go)

### edge-ui (SvelteKit + Tailwind + shadcn-svelte) — embedded in edge-gateway
- Endpoint CRUD
- Webhook history viewer with payload inspection
- Replay trigger
- Queue status dashboard (counts, connection status)
- Settings page (view-only, config via env vars)

### home-hub (Go) — separate container on home network
- Initiates persistent gRPC streaming connection to edge-gateway
- Receives webhooks from edge via stream
- Forwards to destination URLs on home network
- Reports delivery status (success/failure) back to edge
- Local retry logic with exponential backoff (1s→1h cap)

---

## API Surface

### Edge REST API (authenticated via GitHub OAuth)

**Endpoints**
- `POST /api/endpoints` — create endpoint
- `GET /api/endpoints` — list endpoints
- `GET /api/endpoints/{id}` — get endpoint details
- `PUT /api/endpoints/{id}` — update endpoint
- `DELETE /api/endpoints/{id}` — delete endpoint

**Webhooks**
- `GET /api/webhooks` — list webhooks (filterable by endpoint, status)
- `GET /api/webhooks/{id}` — get webhook details + payload
- `POST /api/webhooks/{id}/replay` — replay webhook

**System**
- `GET /api/status` — queue depth, home-hub connection status
- `GET /api/settings` — get settings

### Webhook Ingestion (public, no auth)
- `POST /h/{endpoint-id}` — receive webhook

### MCP Tools
- `hookly_list_endpoints` — list all endpoints
- `hookly_get_endpoint` — get endpoint details
- `hookly_create_endpoint` — create new endpoint
- `hookly_delete_endpoint` — delete endpoint
- `hookly_mute_endpoint` — temporarily disable endpoint
- `hookly_list_webhooks` — list webhooks with filters
- `hookly_get_webhook` — get webhook details + full payload
- `hookly_replay_webhook` — replay a webhook
- `hookly_get_status` — queue depth, connection status

---

## Data Model

### Endpoint
```
id: string (nanoid)
name: string
provider_type: enum (stripe, github, telegram, generic)
signature_secret: string (encrypted at rest)
destination_url: string
created_at: timestamp
updated_at: timestamp
muted: boolean
```

### Webhook
```
id: string (nanoid)
endpoint_id: string (FK)
received_at: timestamp
headers: json
payload: blob
signature_valid: boolean
status: enum (pending, delivered, failed, dead_letter)
attempts: integer
last_attempt_at: timestamp
delivered_at: timestamp
error_message: string
```

### Edge→Home Envelope (protobuf)
```
message WebhookEnvelope {
  string id = 1;
  string endpoint_id = 2;
  string destination_url = 3;
  google.protobuf.Timestamp received_at = 4;
  map<string, string> headers = 5;
  bytes payload = 6;
  int32 attempt = 7;
}
```

---

## Security

- **Webhook verification**: Per-provider signature schemes at edge
- **Edge↔Home auth**: Pre-shared HMAC secret + timestamp (±5 min window)
- **UI/API auth**: GitHub OAuth, authorized by org membership OR username allowlist
- **Secrets at rest**: Signature secrets encrypted in SQLite (AES-256-GCM with key from env)

---

## Behavior Rules

### Delivery
- **In-order per-endpoint**: Same endpoint = arrival order. Different endpoints independent.
- **Success**: 2xx response = delivered
- **Permanent failure**: 4xx response = failed, no retry
- **Transient failure**: 5xx response = retry with exponential backoff

### Retry Strategy
- Exponential backoff: 1s, 2s, 4s, 8s... max 1 hour between retries
- After 7 days undelivered → dead_letter

### Retention
- Buffer duration: 7 days (undelivered webhooks)
- Retention after delivery: 7 days (for replay/audit)

### Invalid Signatures
- Accept but mark `signature_valid=false`
- Store for inspection, don't reject

### Notifications
- Telegram message on delivery failure (after retries exhausted)
- Include endpoint name and error

---

## Configuration

All configuration via environment variables:

```
# Database
DATABASE_PATH=/data/hookly.db
ENCRYPTION_KEY=<32-byte-hex>

# GitHub OAuth
GITHUB_CLIENT_ID=<client-id>
GITHUB_CLIENT_SECRET=<client-secret>
GITHUB_ORG=<org-name>              # Optional: require org membership
GITHUB_ALLOWED_USERS=user1,user2   # Optional: allowlist

# Telegram Notifications
TELEGRAM_BOT_TOKEN=<bot-token>
TELEGRAM_CHAT_ID=<chat-id>

# Home Hub Connection
HOME_HUB_SECRET=<pre-shared-secret>

# Server
PORT=8080
BASE_URL=https://hooks.dx314.com
```

---

## Deployment

### Edge (systemd + Docker Compose)

```yaml
# docker-compose.yml
services:
  edge-gateway:
    image: ghcr.io/youruser/hookly-edge:latest
    ports:
      - "8080:8080"
    volumes:
      - ./data:/data
    environment:
      - DATABASE_PATH=/data/hookly.db
      - ENCRYPTION_KEY=${ENCRYPTION_KEY}
      - GITHUB_CLIENT_ID=${GITHUB_CLIENT_ID}
      - GITHUB_CLIENT_SECRET=${GITHUB_CLIENT_SECRET}
      - GITHUB_ORG=${GITHUB_ORG}
      - TELEGRAM_BOT_TOKEN=${TELEGRAM_BOT_TOKEN}
      - TELEGRAM_CHAT_ID=${TELEGRAM_CHAT_ID}
      - HOME_HUB_SECRET=${HOME_HUB_SECRET}
```

Caddy proxies `hooks.dx314.com` → `localhost:8080`

### Home (Docker Compose)

```yaml
services:
  home-hub:
    image: ghcr.io/youruser/hookly-home:latest
    environment:
      - EDGE_URL=https://hooks.dx314.com
      - HOME_HUB_SECRET=${HOME_HUB_SECRET}
    restart: unless-stopped
```

---

## Decisions Log

| # | Decision | Rationale |
|---|----------|-----------|
| 1 | Operational complexity: **Moderate** | Basic dashboard, push/email alerts, structured logs locally, manual backup/restore |
| 2 | Edge server: Ubuntu, static IP, Caddy handles TLS externally | Caddy out of scope. Edge exposes HTTP; Caddy proxies. |
| 3 | Home server: Ubuntu, Docker Compose/Podman, single server | Keep efficient, no hard constraints |
| 4 | Private link: **App-level only (no VPN)** | Home-hub dials out to edge over public internet. Auth in app layer. |
| 5 | Domain: `hooks.dx314.com` | Single domain for all webhooks |
| 6 | URL model: **Generated unique URLs** | User creates endpoint → system generates URL → user pastes into provider |
| 7 | Delivery: **In-order, per-endpoint** | Same endpoint = arrival order. Different endpoints independent. |
| 8 | Endpoint config: name, provider type, signature secret, destination URL | All required at creation |
| 9 | MVP providers: Stripe, GitHub, Telegram, Generic | Extensible post-MVP |
| 10 | Max payload: **100MB** | Large limit for flexibility |
| 11 | Expected rate: **Tiny (dev use)** | Not designing for high throughput |
| 12 | Invalid signature handling: **Accept but mark unverified** | Store for inspection, don't reject |
| 13 | Edge↔Home auth: **Pre-shared secret (HMAC)** | Simple, no expiry management. Secret generated at setup. |
| 14 | UI/API auth: **GitHub OAuth** | External IdP, SSO-capable |
| 15 | Authorization: **GitHub org membership OR allowlisted users** | Org members OR specific GitHub usernames in allowlist |
| 16 | Notifications: **Telegram integration** | Send alerts to Telegram bot/chat. No native app needed. |
| 17 | Alert triggers: **Delivery failures only** | Notify when webhook fails to deliver after retries |
| 18 | Buffer duration: **7 days** | Webhooks undelivered after 7 days → dead-letter |
| 19 | Retention after delivery: **7 days** | Successful webhooks kept for replay/audit for 7 days |
| 20 | Edge storage: **SQLite** | Single file, zero ops, fits low-volume dev use |
| 21 | Retry strategy: **Exponential backoff (1s→1h cap)** | 1s, 2s, 4s, 8s... max 1 hour between retries |
| 22 | Delivery success: **2xx=success, 4xx=permanent fail, 5xx=retry** | 4xx stops retrying (client error). 5xx keeps retrying (server error). |
| 23 | Edge upgrades: **Manual pull + restart** | `docker compose pull && docker compose up -d` |
| 24 | MCP interface: **Full CRUD** | list/get/replay + create/delete/mute endpoints |
| 25 | MCP payload redaction: **None** | Full payload visible to LLM |
| 26 | MVP scope: **Full feature set** | Core relay + Telegram alerts + MCP + replay |
| 27 | Connection protocol: **gRPC streaming (ConnectRPC)** | Using ConnectRPC + protobuf + buf gen stack. buf v2 config, Go + TS codegen. |
| 28 | Idempotency: **Destination handles it** | No deduplication at edge. Accept all, let destination app be idempotent. |
| 29 | Setup flow: **Environment variables only** | All config via env vars in compose file. No setup wizard. |
| 30 | Dashboard: **Basic counts in UI** | Queue depth, webhooks stats, connection status. Simple stats page. No Prometheus. |
| 31 | UI stack: **SvelteKit + Tailwind + shadcn-svelte** | Modern component library. |
| 32 | UI serving: **Embedded in Go binary** | Build SvelteKit as static, embed in edge-gateway. Single container. |
