# Hookly

Webhook relay for private networks. External services POST to a public edge server, which pushes webhooks to your local network over a persistent gRPC stream. No VPN, no port forwarding.

## Problem

You run services on a private network (Home Assistant, Jellyfin, self-hosted apps). Stripe, GitHub, Telegram want to send you webhooks. You don't want to expose your network to the internet or maintain a VPN.

## Solution

```
Stripe/GitHub/etc → edge-gateway (public) ←── hookly CLI (private) → local services
                         ↑                         ↑
                    accepts webhooks          initiates connection
                    stores in queue           forwards locally
```

The CLI opens an outbound connection to the edge. Webhooks flow through that stream. Your firewall stays closed.

## Features

- **Signature verification**: Provider presets (Stripe, GitHub, Telegram) plus flexible HMAC-SHA256/SHA1, static tokens, and timestamped signatures for any service.
- **Retry with backoff**: 1s → 1h cap, 7 days before dead-letter. 4xx = permanent fail, 5xx = retry.
- **Fan-out**: One endpoint delivers to 1..N destinations (e.g. a Telegram bot, which only allows a single webhook URL, feeding several local services). Delivery, retry and dead-lettering are tracked per destination, so one listener being down never blocks or re-delivers to the others.
- **In-order delivery**: Ordering per (endpoint, destination). Endpoints and destinations are independent.
- **Web UI**: Dashboard, endpoint management, webhook inspection, replay failed deliveries, theme customization.
- **MCP tools**: Full API for LLM assistants (list endpoints, replay webhooks, check queue depth).
- **Telegram alerts**: Notifications when deliveries hit dead-letter.
- **Run as service**: Install and manage as a system service (systemd/launchd).

## Hosted Service

A free hosted instance is available at **https://hooks.dx314.com**:

- No server setup required
- GitHub authentication
- Full web UI for managing endpoints and inspecting webhooks
- MCP tools for LLM integration

Just install the CLI, authenticate with GitHub, and start receiving webhooks.

## Quick Start

### 1. Install the CLI

```bash
go install hooks.dx314.com/hookly@latest
```

### 2. Authenticate

```bash
hookly login
```

Opens a browser for GitHub OAuth. Credentials are encrypted and stored locally.

### 3. Create an endpoint

Visit **https://hooks.dx314.com** and create an endpoint. Select your provider (Stripe, GitHub, Telegram, Generic, or Custom) and set the destination URL.

### 4. Configure

```bash
hookly init
```

Interactive wizard that creates `hookly.yaml`:

```yaml
edge_url: "https://hooks.dx314.com"
# hub_id is optional - auto-generated from hostname if empty

endpoints:
  - id: "ep_abc123def456"
    destination: "http://localhost:3000/webhooks/stripe"
```

### 5. Run

```bash
hookly
```

That's it. Webhooks flow to your local service.

## CLI Commands

| Command | Description |
|---------|-------------|
| `hookly` | Start the relay (default action) |
| `hookly login` | Authenticate via GitHub OAuth |
| `hookly logout` | Clear stored credentials |
| `hookly whoami` | Show current user |
| `hookly status` | Show connection and config status |
| `hookly init` | Create hookly.yaml interactively |
| `hookly sync` | Create/update endpoints and destinations on the edge from hookly.yaml (`--dry-run`) |
| `hookly install` | Install and start a user service for `./hookly.yaml`, named after its directory (no sudo; `hookly uninstall` removes it) |
| `hookly version` | Print the version and exact build |
| `hookly service list --user` | List installed hookly services |
| `hookly service install` | Install as system service |
| `hookly service start` | Start the service |
| `hookly service stop` | Stop the service |
| `hookly service status` | Show service status |
| `hookly service logs` | View service logs |

All `hookly service` commands take `--name` to pick a named service.

### Several relays on one machine

Each directory with a `hookly.yaml` gets its own service: `hookly install` in
`~/homeboy` installs `hookly-homeboy`, in `~/schoolboy` installs
`hookly-schoolboy` (use `--name` to choose another name). Each runs with hub ID
`<hostname>-<name>` unless `hub_id` is set.

The edge sends each endpoint to one relay, so the relays must relay different
endpoints: `hookly install` refuses a second service for an endpoint another one
already has, and the edge rejects a relay whose endpoint a live relay holds. To
deliver one endpoint to several local services, add them as destinations of that
endpoint and list them in one `hookly.yaml`.

## Configuration

### hookly.yaml

`hookly.yaml` declares your endpoints and destinations; hookly makes the edge
match it. On start (and with `hookly sync`, or `hookly install`) it creates
endpoints and destinations that don't exist, updates what differs, and writes
the `id` and public `url` the edge assigns back into the file, keeping your
comments. Settings the file leaves out are left as they are on the edge.

```yaml
# Required: edge server URL
edge_url: "https://hooks.dx314.com"

# Optional: unique identifier (defaults to hostname; <hostname>-<name> for a named service)
hub_id: "my-server"

endpoints:
  - name: "telegram-bot"            # identifies the endpoint until it has an id
    id: "Hzy7Ds64..."               # filled in by hookly
    url: "https://hooks.dx314.com/h/Hzy7Ds64..."  # filled in: give this to the provider
    provider: telegram              # stripe | github | telegram | generic | custom
    secret_env: BOT_WEBHOOK_SECRET  # or secret: "..." (Telegram: the setWebhook secret_token)
    destinations:                   # the first is the primary when created
      - name: schoolboy
        url: "http://127.0.0.1:8789/telegram"
      - name: homeboy
        url: "http://127.0.0.1:8790/telegram"
        enabled: true               # default
    # prune: true                   # also remove edge destinations not listed here
    # muted: false

  - name: "acme"
    provider: custom
    secret: "..."
    verification:
      method: hmac_sha256           # static | hmac_sha256 | hmac_sha1 | timestamped_hmac
      signature_header: X-Acme-Signature
      signature_prefix: "sha256="
    destinations:
      - name: default
        url: "http://localhost:3000/webhooks/acme"
```

- **Identity**: an endpoint with an `id` is that endpoint; a wrong `id` is an
  error, never a new endpoint. Without an `id`, hookly adopts the endpoint with
  that `name` (several with the same name is an error) or creates it.
- **Secrets** can't be read back from the edge, so a `secret`/`secret_env` is
  sent on every sync. Without one, the edge's secret is kept.
- **Destinations** are matched by name. Ones on the edge but not in the file are
  left alone (logged) unless `prune: true`.
- The older forms still work: `destinations:` as a `name: url` map, and
  `destination:` for the primary destination's URL.
- `hookly sync --dry-run` shows what would change.

### Reverse proxy (`proxies:`)

The relay can also serve a local web app to the internet through the same stream, like a small Cloudflare Tunnel. Add to `hookly.yaml`:

```yaml
proxies:
  - name: "homeboy"
    url: "http://127.0.0.1:8790"
    paths: ["/app/"]   # only these prefixes are forwarded; anything else answers 404
```

The app is then reachable at `https://hooks.dx314.com/p/<hub_id>/homeboy/app/...` for any HTTP method. The `/p/<hub_id>/homeboy` prefix is stripped, so the app sees `/app/...`; it also gets `X-Forwarded-For`, `-Proto`, `-Host` and `X-Forwarded-Prefix`. Hop-by-hop headers are dropped both ways, redirects are passed back as-is, and request and response bodies are capped at 10 MiB. Without a `proxies:` section nothing is ever proxied; the request can never pick the upstream host, and `..` (encoded or not) is rejected at both ends.

The edge verifies nothing and stores nothing for these requests: **the app must do its own authentication** (a Telegram Mini App verifies `initData`). The edge answers `502` when that hub is not connected, `504` when the app does not answer within `PROXY_TIMEOUT` (default 45s, so a long-poll of ~25s fits) and `503` when the hub already has 128 requests in flight. The CLI forwards up to 64 requests at once, independently of webhook delivery. Streaming responses and WebSockets are not supported.

**Upgrading**: deploy the edge first, then the CLI. An older CLI simply never advertises the `proxy` capability and is never sent a request; an older edge ignores the `proxies:` list.

### Multiple destinations

An endpoint starts with one destination and can have more (UI: endpoint → Edit → Destinations; API: `AddDestination` / `UpdateDestination` / `RemoveDestination`; MCP: `hookly_add_destination` etc.).

- The signature is verified once, at the edge. Every destination receives the original headers and body.
- The provider always gets an immediate `200` once the webhook is stored; it never waits for delivery.
- A destination added later only receives webhooks that arrive after it was added. Replay can target it explicitly.
- Removing a destination abandons its pending deliveries (its delivery history is deleted with it). The last destination can't be removed; mute the endpoint instead.
- Disabling a destination pauses it: new webhooks skip it and its pending deliveries are held.
- A webhook's status is derived: `delivered` when every enabled destination is delivered, `failed` / `dead_letter` if any destination is, otherwise `pending`. Replay can target one destination or all.
- The first destination is the *primary*. `destination_url` in the API/MCP still works and means the primary destination.

**Upgrading**: deploy the edge first. Migration 007 runs on start (one destination per existing endpoint, one delivery per existing webhook; the legacy `endpoints.destination_url` column is kept and mirrors the primary destination). A `hookly` CLI that predates fan-out keeps working for the primary destination of every endpoint; additional destinations stay pending until the CLI is upgraded and advertises the `fanout` capability.

### Files

| Path | Description |
|------|-------------|
| `~/.config/hookly/credentials.json` | Encrypted auth credentials |
| `./hookly.yaml` | Endpoint (and proxy) configuration |

## Signature Verification

Hookly verifies webhook signatures for known providers:

| Provider | Header | Format |
|----------|--------|--------|
| **Stripe** | `Stripe-Signature` | `t=timestamp,v1=hmac` |
| **GitHub** | `X-Hub-Signature-256` | `sha256=hmac` |
| **Telegram** | `X-Telegram-Bot-Api-Secret-Token` | secret token |
| **Generic** | `X-Webhook-Signature` | `sha256=hmac` |

### Custom Verification

For other providers, create an endpoint with provider type "custom" and configure verification:

```json
{
  "method": "hmac_sha256",
  "signature_header": "X-My-Signature",
  "signature_prefix": "sha256=",
  "timestamp_header": "X-Timestamp",
  "timestamp_tolerance": 300
}
```

Methods: `hmac_sha256`, `hmac_sha1`, `static`, `timestamped_hmac`

**Note**: Invalid signatures are logged but NOT rejected. Webhooks are always stored for inspection and replay.

## Edge Gateway (Self-Hosted)

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `DATABASE_PATH` | Yes | SQLite file path |
| `ENCRYPTION_KEY` | Yes | 32-byte hex for encrypting secrets at rest |
| `PORT` | No | Default 8080 |
| `BASE_URL` | No | Public URL for webhook endpoints |
| `GITHUB_CLIENT_ID` | No | OAuth for UI login |
| `GITHUB_CLIENT_SECRET` | No | OAuth for UI login |
| `GITHUB_ORG` | No | Restrict to org members |
| `GITHUB_ALLOWED_USERS` | No | Comma-separated allowlist |
| `TELEGRAM_BOT_TOKEN` | No | Failure notifications |
| `TELEGRAM_CHAT_ID` | No | Failure notifications |

### Docker

```bash
docker build -f deploy/edge/Dockerfile -t hookly-edge .
```

```yaml
services:
  edge:
    image: hookly-edge:latest
    ports: ["8080:8080"]
    volumes: ["./data:/data"]
    environment:
      DATABASE_PATH: /data/hookly.db
      ENCRYPTION_KEY: ${ENCRYPTION_KEY}
      BASE_URL: https://hooks.example.com
```

Put a reverse proxy (Caddy, nginx) in front for TLS.

## Web UI

- **Dashboard**: Queue stats (pending, failed, dead-letter), connected endpoints
- **Endpoints**: Create, edit, delete. Copy webhook URLs. Mute/unmute.
- **Webhooks**: Filter by endpoint/status, view full payload and headers, replay failed deliveries
- **Settings**: Theme selection, Telegram notification config

## MCP Tools

For LLM integration, run `hookly-mcp` as a stdio tool server:

```bash
go run ./cmd/hookly-mcp
```

Available tools:

| Tool | Description |
|------|-------------|
| `hookly_list_endpoints` | List all endpoints with webhook URLs |
| `hookly_get_endpoint` | Get endpoint details |
| `hookly_create_endpoint` | Create endpoint with provider and secret |
| `hookly_delete_endpoint` | Delete endpoint and its webhooks |
| `hookly_mute_endpoint` | Mute/unmute webhook reception |
| `hookly_add_destination` | Add a destination to an endpoint |
| `hookly_update_destination` | Rename, re-point, enable or pause a destination |
| `hookly_remove_destination` | Remove a destination (abandons its pending deliveries) |
| `hookly_list_webhooks` | Filter by endpoint/status, pagination |
| `hookly_get_webhook` | Full payload, headers, per-destination delivery status |
| `hookly_replay_webhook` | Reset webhook for redelivery (all destinations, or one) |
| `hookly_get_status` | Queue depth and connected endpoints |

Uses CLI credentials from `hookly login`.

## API

ConnectRPC at `/hookly.v1.EdgeService/`. See `proto/hookly/v1/edge.proto`.

## Project Structure

```
hookly/               # CLI (go install target)
cmd/
  edge-gateway/       # Public edge server
  hookly-mcp/         # MCP tool server
internal/
  webhook/            # Ingestion, verification, forwarding
  relay/              # gRPC stream, dispatcher
  auth/               # GitHub OAuth, sessions, tokens
  cli/                # CLI commands, credentials, wizard
  mcp/                # MCP server and tools
  db/                 # SQLite, migrations, encryption
frontend/             # SvelteKit UI (embedded in binary)
proto/                # Protocol definitions
sql/                  # Schema and queries (sqlc)
```

## License

MIT
