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

- **Signature verification**: Built-in support for Stripe, GitHub, Telegram. Custom verification for any provider.
- **Retry with backoff**: 1s → 1h cap, 7 days before dead-letter. 4xx = permanent fail, 5xx = retry.
- **In-order delivery**: Per-endpoint ordering. Endpoints are independent.
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
| `hookly service install` | Install as system service |
| `hookly service start` | Start the service |
| `hookly service stop` | Stop the service |
| `hookly service status` | Show service status |
| `hookly service logs` | View service logs |

## Configuration

### hookly.yaml

```yaml
# Required: edge server URL
edge_url: "https://hooks.dx314.com"

# Optional: unique identifier (defaults to hostname)
hub_id: "my-server"

# Endpoints this client handles
endpoints:
  - id: "ep_abc123"
    # Optional: override the destination URL (uses edge-configured if omitted)
    destination: "http://localhost:8080/webhook"
  - id: "ep_def456"
    # No destination - uses what's configured on the edge
```

### Files

| Path | Description |
|------|-------------|
| `~/.config/hookly/credentials.json` | Encrypted auth credentials |
| `./hookly.yaml` | Endpoint configuration |

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
| `hookly_list_webhooks` | Filter by endpoint/status, pagination |
| `hookly_get_webhook` | Full payload, headers, attempt count |
| `hookly_replay_webhook` | Reset webhook for redelivery |
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
