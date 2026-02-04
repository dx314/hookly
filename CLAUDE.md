# Hookly

Webhook relay: public edge → private home network. No VPN.

## Quick Start

```bash
buf generate          # proto → Go/TS
sqlc generate         # SQL → Go
make all              # build everything
go run ./cmd/edge-gateway
go run ./cmd/home-hub
```

## Architecture

```
External → edge-gateway (public) ←gRPC stream← home-hub (private) → local services
```

**Data flow**: POST `/h/{id}` → verify sig → store → push to home-hub → forward → ACK

## Key Files

| Area | Files |
|------|-------|
| **Entrypoints** | `cmd/edge-gateway/main.go`, `cmd/home-hub/main.go`, `cmd/hookly-mcp/main.go` |
| **Proto** | `proto/hookly/v1/{common,edge,relay}.proto` |
| **Schema** | `sql/schema.sql`, `sql/queries/*.sql` |
| **Webhook** | `internal/webhook/{handler,verify,forwarder,scheduler,backoff}.go` |
| **Relay** | `internal/relay/{handler,client,dispatcher,manager,auth}.go` |
| **Auth** | `internal/auth/{github,session,authorize,handlers}.go` |
| **API** | `internal/service/edge/service.go` (ConnectRPC) |
| **Config** | `internal/config/{config,home}.go` |
| **MCP** | `internal/mcp/{server,tools}.go` |
| **Frontend** | `frontend/src/routes/**/*.svelte` |

## Patterns

- **IDs**: nanoid (not UUID)
- **Secrets**: AES-256-GCM encrypted at rest (`internal/crypto/aes.go`)
- **Logging**: `log/slog` structured
- **Router**: chi/v5
- **API**: ConnectRPC + protobuf
- **Auth**: GitHub OAuth, org/user allowlist
- **Retry**: exponential backoff 1s→1h, dead-letter after 7d

## Env Vars

**Edge**: `DATABASE_PATH`, `ENCRYPTION_KEY`, `PORT`, `BASE_URL`, `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`, `GITHUB_ORG`, `GITHUB_ALLOWED_USERS`, `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`, `HOME_HUB_SECRET`

**Home**: `EDGE_URL`, `HOME_HUB_SECRET`, `HUB_ID`

## References

For patterns, see `/home/alex/src/aura/`:
- `buf.yaml`, `buf.gen.yaml` — buf config
- `backend/cmd/api/main.go` — ConnectRPC setup
- `backend/internal/server/interceptors.go` — auth interceptor

## Spec

Full spec in `SPEC.md`. Work orders in `works/` (all DONE).
