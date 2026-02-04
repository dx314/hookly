# Hookly

Webhook relay for private networks. External services POST to a public edge server, which pushes webhooks to your home network over a persistent gRPC stream. No VPN, no port forwarding.

## Problem

You run services on a private network (Home Assistant, Jellyfin, whatever). Stripe, GitHub, Telegram want to send you webhooks. You don't want to expose your network to the internet or maintain a VPN.

## Solution

```
Stripe/GitHub/etc → edge-gateway (public) ←── home-hub (private) → local services
                         ↑                         ↑
                    accepts webhooks          initiates connection
                    stores in queue           forwards locally
```

The home-hub opens an outbound connection to the edge. Webhooks flow through that stream. Your firewall stays closed.

## Features

- **Provider verification**: Stripe, GitHub, Telegram signatures checked. Unverified webhooks stored (not rejected) for inspection.
- **Retry with backoff**: 1s → 1h cap, 7 days before dead-letter. 4xx = permanent fail, 5xx = retry.
- **In-order delivery**: Per-endpoint ordering. Endpoints are independent.
- **Multi-hub**: Different home-hubs can handle different endpoints.
- **Web UI**: Dashboard, endpoint management, webhook inspection, replay failed deliveries.
- **MCP tools**: Full API for LLM assistants (list endpoints, replay webhooks, check queue depth).
- **Telegram alerts**: Notifications when deliveries fail.

## Quick Start

```bash
# Generate code from protos and SQL
buf generate
sqlc generate

# Build
make all

# Run edge (public server)
DATABASE_PATH=./hookly.db \
ENCRYPTION_KEY=$(openssl rand -hex 32) \
go run ./cmd/edge-gateway

# Run home-hub (private network)
EDGE_URL=https://hooks.example.com \
HOME_HUB_SECRET=<same-as-edge> \
HUB_ID=home-1 \
go run ./cmd/home-hub
```

## Configuration

### Edge Gateway

| Variable | Required | Description |
|----------|----------|-------------|
| `DATABASE_PATH` | Yes | SQLite file path |
| `ENCRYPTION_KEY` | Yes | 32-byte hex for encrypting secrets at rest |
| `PORT` | No | Default 8080 |
| `BASE_URL` | No | Public URL for webhook endpoints |
| `HOME_HUB_SECRET` | No | Pre-shared secret for home-hub auth |
| `GITHUB_CLIENT_ID` | No | OAuth for UI login |
| `GITHUB_CLIENT_SECRET` | No | OAuth for UI login |
| `GITHUB_ORG` | No | Restrict to org members |
| `GITHUB_ALLOWED_USERS` | No | Comma-separated allowlist |
| `TELEGRAM_BOT_TOKEN` | No | Failure notifications |
| `TELEGRAM_CHAT_ID` | No | Failure notifications |

### Home Hub

| Variable | Required | Description |
|----------|----------|-------------|
| `EDGE_URL` | Yes | Edge gateway URL |
| `HOME_HUB_SECRET` | Yes | Must match edge |
| `HUB_ID` | Yes | Unique identifier for this hub |

## Deployment

### Docker

```bash
docker build -f deploy/edge/Dockerfile -t hookly-edge .
docker build -f deploy/home/Dockerfile -t hookly-home .
```

### Docker Compose (Edge)

```yaml
services:
  edge:
    image: hookly-edge:latest
    ports: ["8080:8080"]
    volumes: ["./data:/data"]
    environment:
      DATABASE_PATH: /data/hookly.db
      ENCRYPTION_KEY: ${ENCRYPTION_KEY}
      HOME_HUB_SECRET: ${HOME_HUB_SECRET}
      BASE_URL: https://hooks.example.com
```

Put a reverse proxy (Caddy, nginx) in front for TLS.

### Docker Compose (Home)

```yaml
services:
  home-hub:
    image: hookly-home:latest
    environment:
      EDGE_URL: https://hooks.example.com
      HOME_HUB_SECRET: ${HOME_HUB_SECRET}
      HUB_ID: home-1
```

## Usage

1. Create an endpoint in the UI (or via MCP)
2. Copy the webhook URL: `https://hooks.example.com/h/{endpoint-id}`
3. Configure your provider (Stripe dashboard, GitHub repo settings, etc.)
4. Webhooks arrive at edge, get pushed to home-hub, forwarded to your local service

## API

ConnectRPC at `/hookly.v1.EdgeService/`. See `proto/hookly/v1/edge.proto`.

**MCP tools** (for LLM integration):
- `list_endpoints`, `get_endpoint`, `create_endpoint`, `delete_endpoint`
- `list_webhooks`, `get_webhook`, `replay_webhook`
- `get_status`

Run `hookly-mcp` as a stdio tool server.

## Project Structure

```
cmd/
  edge-gateway/     # Public edge server
  home-hub/         # Private network client
  hookly-mcp/       # MCP tool server
internal/
  webhook/          # Ingestion, verification, scheduling
  relay/            # gRPC stream, dispatcher, hub management
  auth/             # GitHub OAuth, sessions
  db/               # SQLite, encryption
frontend/           # SvelteKit UI (embedded in binary)
proto/              # Protocol definitions
sql/                # Schema and queries (sqlc)
```

## License

MIT
