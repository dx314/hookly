# Hookly

Webhook relay: public edge → private home network. No VPN.

## Quick Start

```bash
buf generate          # proto → Go/TS
sqlc generate         # SQL → Go
make all              # build everything
go run ./cmd/edge-gateway
hookly login && hookly  # run CLI relay
```

## Architecture

```
External → edge-gateway (public) ←gRPC stream← hookly CLI (private) → local services
```

**Data flow**: POST `/h/{id}` → verify sig → store → push to CLI → forward → ACK

## Key Files

| Area | Files |
|------|-------|
| **Entrypoints** | `cmd/edge-gateway/main.go`, `hookly/main.go` (CLI), `cmd/hookly-mcp/main.go` |
| **Proto** | `proto/hookly/v1/{common,edge,relay}.proto` |
| **Schema** | `sql/schema.sql`, `sql/queries/*.sql`, `internal/db/migrations/*.sql` |
| **Webhook** | `internal/webhook/{handler,verify,forwarder,scheduler,backoff}.go` |
| **Relay** | `internal/relay/{handler,client,dispatcher,manager}.go` |
| **Auth** | `internal/auth/{github,session,authorize,handlers}.go` |
| **API** | `internal/service/edge/service.go` (ConnectRPC) |
| **Config** | `internal/config/{config,hookly}.go` |
| **CLI** | `internal/cli/{credentials,login,wizard,client}.go` |
| **MCP** | `internal/mcp/{server,tools}.go` |
| **Frontend** | `frontend/src/routes/**/*.svelte` |

## Migrations

Uses [goose](https://github.com/pressly/goose) with embedded SQL migrations. Path from `DATABASE_PATH` env (default: `./hookly.db`).

```bash
make migrate-status      # show migration status
make migrate-up          # apply pending migrations
make migrate-down        # rollback one migration
make migrate-create NAME=add_foo  # create new migration
make dump-schema         # dump schema from DB
```

Migrations run automatically on startup. Files in `internal/db/migrations/`.

## Patterns

- **IDs**: nanoid (not UUID)
- **Secrets**: AES-256-GCM encrypted at rest (`internal/crypto/aes.go`)
- **Logging**: `log/slog` structured
- **Router**: chi/v5
- **API**: ConnectRPC + protobuf
- **Auth**: GitHub OAuth, bearer tokens, org/user allowlist
- **Retry**: exponential backoff 1s→1h, dead-letter after 7d
- **Verification**: Stripe, GitHub, Telegram built-in + custom schemes

## Env Vars

**Edge**: `DATABASE_PATH`, `ENCRYPTION_KEY`, `PORT`, `BASE_URL`, `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`, `GITHUB_ORG`, `GITHUB_ALLOWED_USERS`, `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`

**MCP**: Uses CLI credentials from `hookly login`. Optional: `DATABASE_PATH`, `ENCRYPTION_KEY`, `BASE_URL`.

**CLI**: Uses bearer token auth (from `hookly login`). Config: `hookly.yaml`, creds: `~/.config/hookly/credentials.json`

## CLI

```bash
go install hooks.dx314.com/hookly@latest
```

Commands: `login`, `logout`, `whoami`, `status`, `init`, `service`
Default (no args): run relay client. Config: `hookly.yaml`, creds: `~/.config/hookly/`

Service subcommands: `install`, `uninstall`, `start`, `stop`, `restart`, `status`, `logs`

## References

For patterns, see `/home/alex/src/aura/`:
- `buf.yaml`, `buf.gen.yaml` — buf config
- `backend/cmd/api/main.go` — ConnectRPC setup
- `backend/internal/server/interceptors.go` — auth interceptor

## Deploy

```bash
./deploy/edge/deploy.sh          # build, push, deploy, cleanup
./deploy/edge/deploy.sh --deploy-only   # just redeploy existing image
```

**Production**: https://hooks.dx314.com
**Coolify**: https://svr.alexdunmow.com
**Registry**: git.dev.alexdunmow.com/alex/hookly/edge

Tokens: `~/.config/coolify/token`, `~/.config/gitea/token`

## Spec

Full spec in `SPEC.md`. Work orders in `works/` (all DONE).
