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

**Data flow**: POST `/h/{id}` → verify sig → store + one delivery per destination → 200 → push each delivery to CLI → forward → ACK per delivery

**Reverse proxy** (`/p/{hub_id}/{name}/*`, any method): the edge turns the request into an `HttpRequest` on the same stream, the CLI forwards it to the local service named under `proxies:` in `hookly.yaml` (only its `paths` prefixes; the `/p/{hub_id}/{name}` prefix is stripped) and returns an `HttpResponse`. Nothing is verified or stored at the edge; the service does its own auth. Only hubs advertising the `proxy` capability and the name are sent requests; 502 if none is connected, 504 after `PROXY_TIMEOUT` (default 45s, min 35s: long-polls), bodies capped at 10 MiB. `internal/proxy/` holds both ends and the shared header/path rules; pending requests live on `relay.HubConnection` and fail on disconnect.

**Fan-out**: endpoint → 1..N `destinations`; `deliveries` (webhook × destination) hold status/attempts/backoff. `webhooks.status` etc. are a derived rollup (`db.Rollup`). All multi-row changes go through `db.Store` (`internal/db/store.go`), not raw queries. `endpoints.destination_url` is a legacy column mirroring the primary (first) destination. CLIs advertise the `fanout` capability; CLIs without it only get primary destinations.

## Key Files

| Area | Files |
|------|-------|
| **Entrypoints** | `cmd/edge-gateway/main.go`, `hookly/main.go` (CLI), `cmd/hookly-mcp/main.go` |
| **Proto** | `proto/hookly/v1/{common,edge,relay}.proto` |
| **Schema** | `sql/schema.sql`, `sql/queries/*.sql`, `internal/db/migrations/*.sql`, `internal/db/{store,rollup}.go` |
| **Webhook** | `internal/webhook/{handler,verify,forwarder,scheduler,backoff}.go` |
| **Relay** | `internal/relay/{handler,client,dispatcher,manager}.go` |
| **Proxy** | `internal/proxy/{headers,local,edge}.go` (shared rules, CLI forwarder, edge `/p/` handler) |
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
- **Retry**: exponential backoff 1s→1h, dead-letter after 7d — per destination
- **Ordering**: in-order per (endpoint, destination); one in-flight delivery per destination
- **SQL params**: don't mix `sqlc.narg()` with bare `?` in one query (SQLite numbers them wrongly) — use `sqlc.arg()`
- **Verification**: Stripe, GitHub, Telegram built-in + custom schemes

## Env Vars

**Edge**: `DATABASE_PATH`, `ENCRYPTION_KEY`, `PORT`, `BASE_URL`, `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`, `GITHUB_ORG`, `GITHUB_ALLOWED_USERS`, `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`, `PROXY_TIMEOUT` (optional, default `45s`)

**MCP**: Uses CLI credentials from `hookly login`. Optional: `DATABASE_PATH`, `ENCRYPTION_KEY`, `BASE_URL`.

**CLI**: Uses bearer token auth (from `hookly login`). Config: `hookly.yaml` (`endpoints:`, optional `proxies:`), creds: `~/.config/hookly/credentials.json`

## CLI

```bash
go install hooks.dx314.com/hookly@latest
```

Commands: `login`, `logout`, `whoami`, `status`, `init`, `sync`, `install`, `uninstall`, `service`
Default (no args): run relay client. Config: `hookly.yaml`, creds: `~/.config/hookly/`

Service subcommands: `list`, `install`, `uninstall`, `start`, `stop`, `restart`, `status`, `logs` (all take `--name`)

**hookly.yaml is declarative**: `internal/provision` makes the edge match it on relay start, `hookly sync` and `hookly install` — creates/updates endpoints (name, provider, secret, custom verification, muted) and destinations (by name; `prune` removes extras), secrets compared via `crypto.SecretFingerprint` (edge returns `signature_secret_fingerprint`), and writes the assigned `id`/`url` back into the file (`config.SetEndpointFields`: yaml node edit, comments kept, temp file + rename).

**Versioning**: `hookly version` comes from git tags only (no version constant). `.github/workflows/tag.yml` tags every push to GitHub `main`: `feat` commit → minor bump, else patch. Never tag or edit versions by hand.

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
