# Hookly Specification

**Status:** FINAL — Ready for implementation

---

## Problem Statement

Route webhooks from external providers (Stripe, GitHub, Telegram, etc.) to services on a home network with no inbound WAN exposure. Must:
- Accept webhooks at public edge (`hooks.dx314.com`)
- Generate unique URLs per endpoint
- Verify signatures (mark unverified if invalid, don't reject)
- Buffer when home offline, deliver later, in-order per (endpoint, destination)
- Fan one endpoint out to several destinations (a Telegram bot has exactly one webhook URL)
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
3a. edge-gateway creates one delivery per enabled destination and answers the provider `200` (never waits for delivery)
4. edge-gateway pushes one envelope per delivery to home-hub via persistent connection
5. home-hub forwards each envelope to its destination URL on home network (one worker per destination)
6. home-hub ACKs each delivery to edge-gateway
7. edge-gateway marks the delivery as delivered and re-derives the webhook's status

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
- `hookly_add_destination` — add a destination to an endpoint
- `hookly_update_destination` — rename / re-point / enable / pause a destination
- `hookly_remove_destination` — remove a destination (abandons its pending deliveries)
- `hookly_list_webhooks` — list webhooks with filters
- `hookly_get_webhook` — get webhook details + full payload
- `hookly_replay_webhook` — replay a webhook (all destinations, or one)
- `hookly_get_status` — queue depth, connection status

---

## Data Model

### Endpoint
```
id: string (nanoid)
name: string
provider_type: enum (stripe, github, telegram, generic)
signature_secret: string (encrypted at rest)
destination_url: string (LEGACY column, mirrors the primary destination's url)
created_at: timestamp
updated_at: timestamp
muted: boolean
```

### Destination (1..N per endpoint)
```
id: string (nanoid)
endpoint_id: string (FK, cascade)
name: string (unique per endpoint; key for hub-side overrides)
url: string
enabled: boolean (disabled = paused: skipped for new webhooks, pending held)
position: integer (lowest = primary destination)
created_at: timestamp
updated_at: timestamp
```

### Webhook
```
id: string (nanoid)
endpoint_id: string (FK)
received_at: timestamp
headers: json
payload: blob
signature_valid: boolean
status: enum (pending, delivered, failed, dead_letter)   -- derived, see below
attempts: integer                                        -- derived (max)
last_attempt_at: timestamp                               -- derived (latest)
delivered_at: timestamp                                  -- derived (latest, when delivered)
error_message: string                                    -- derived (from the deciding delivery)
```

### Delivery (one per webhook × destination)
```
seq: integer (autoincrement; arrival order per destination)
id: string
webhook_id: string (FK, cascade)
destination_id: string (FK, cascade)
status: enum (pending, delivered, failed, dead_letter)
attempts: integer
last_attempt_at: timestamp
delivered_at: timestamp
error_message: string
notification_sent: boolean
created_at: timestamp
```

Delivery state machine: `pending → delivered` (2xx), `pending → failed` (4xx),
`pending → pending, attempts+1` (5xx / network, retried after backoff),
`pending → dead_letter` (webhook older than 7 days), `any → pending` (replay).
Acks for deliveries that are not pending are ignored.

Derived webhook status (over deliveries to enabled destinations): `dead_letter` if any
delivery is, else `failed` if any is, else `pending` if any is, else `delivered`.

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
  string delivery_id = 8;        // fan-out: echoed in DeliveryAck
  string destination_id = 9;
  string destination_name = 10;  // key for hub-side overrides
  bool destination_primary = 11;
}
```

`DeliveryAck` gained `delivery_id = 6` / `destination_id = 7`, `ConnectRequest` gained
`repeated string capabilities = 5` (`"fanout"`; 4 is skipped, it was `endpoint_ids` before bearer-token auth). No field was renumbered or removed.
Hubs without the `fanout` capability are only sent each endpoint's primary destination and
their acks (webhook ID only) resolve to the webhook's first pending delivery.

---

## Security

- **Webhook verification**: Per-provider signature schemes at edge
- **Edge↔Home auth**: Pre-shared HMAC secret + timestamp (±5 min window)
- **UI/API auth**: GitHub OAuth, authorized by org membership OR username allowlist
- **Secrets at rest**: Signature secrets encrypted in SQLite (AES-256-GCM with key from env)

---

## Behavior Rules

### Delivery
- **In-order per (endpoint, destination)**: Same destination = arrival order. Different endpoints and different destinations of one endpoint are independent: a failing destination never blocks or re-delivers to another.
- **Fan-out at arrival**: a webhook gets one delivery per destination enabled when it arrives. Destinations added later get no old traffic. Removing a destination abandons (deletes) its deliveries.
- **At most one in flight per destination**: a sent delivery is not sent again until it is acked, the hub disconnects, or 90s pass.
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
- Include endpoint name, destination name/URL and error (one notification per failed delivery)

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
| 33 | Fan-out: **1..N destinations per endpoint, delivery state per destination** | A Telegram bot has one webhook URL but several home services need its updates. `deliveries` table (webhook × destination) carries status/attempts/backoff; supersedes #7 (ordering is now per endpoint+destination) and #8 (destination URL → list of named destinations). |
| 34 | Webhook status: **Derived rollup stored on the webhook row** | Keeps list/dashboard/retention queries unchanged and lets an older binary still read the table. Worst status wins: dead_letter > failed > pending > delivered. |
| 35 | Migration: **In place, additive, no table rebuild** | Goose migration 007: new tables + back-fill (one destination per endpoint, one delivery per webhook). Legacy `endpoints.destination_url` stays and mirrors the primary destination, so rollback to an older binary works. |
| 36 | Wire compat: **New proto fields only + `fanout` hub capability** | Edge and hub deploy separately. Old hubs get only primary destinations and ack by webhook ID; extra destinations wait for an upgraded hub. Safe order: edge first, then hub. |
| 37 | Late destinations: **No back-fill; removal abandons** | A destination added later only sees new webhooks (explicit replay can target it). Removing one deletes its deliveries; the last destination can't be removed. |
| 38 | Disabled destination: **Paused** | Skipped for new webhooks, pending deliveries held, excluded from the derived status. At least one destination must stay enabled (mute the endpoint instead). |
| 39 | Hub overrides: **Per destination name; legacy `destination:` = primary only** | A per-endpoint override must never capture a second destination's traffic. |
