# Hookly Work Orders

## Swimlanes

```
INFRASTRUCTURE    BACKEND           FRONTEND          INTEGRATION       DEVOPS
──────────────    ───────           ────────          ───────────       ──────
00-foundation ─┬─► 01-database ─┬─► 08-frontend ─►    10-notifications   12-deployment
               │                │   09-embed          11-mcp
               │   02-edge-core ◄┘
               │        │
               └─► 03-relay ────► 04-home-hub
                       │
                       └─► 05-retry

               06-auth ──► 07-api
```

## Dependency Graph

```
00-foundation
├── 01-database
│   ├── 02-edge-core
│   │   └── 06-auth
│   │       └── 07-api
│   │           ├── 08-frontend
│   │           │   └── 09-embed
│   │           └── 11-mcp
│   ├── 03-relay
│   │   ├── 04-home-hub
│   │   └── 05-retry
│   │       └── 10-notifications
│   └── 12-deployment (depends on all)
```

## Work Order Summary

| # | Name | Swimlane | Status | Dependencies |
|---|------|----------|--------|--------------|
| 00 | Foundation | Infrastructure | NOT STARTED | - |
| 01 | Database | Backend | NOT STARTED | 00 |
| 02 | Edge Core | Backend | NOT STARTED | 00, 01 |
| 03 | Relay Service | Backend | NOT STARTED | 00, 01, 02 |
| 04 | Home Hub | Backend | NOT STARTED | 00, 03 |
| 05 | Retry Logic | Backend | NOT STARTED | 01, 03, 04 |
| 06 | Authentication | Backend | NOT STARTED | 00, 02 |
| 07 | API Layer | Backend | NOT STARTED | 01, 06 |
| 08 | Frontend | Frontend | NOT STARTED | 07 |
| 09 | Embed Frontend | Integration | NOT STARTED | 08 |
| 10 | Notifications | Integration | NOT STARTED | 05 |
| 11 | MCP Server | Integration | NOT STARTED | 07 |
| 12 | Deployment | DevOps | NOT STARTED | All |

## Suggested Execution Order

### Phase 1: Core Backend (can parallelize 02+06)
1. **00-foundation** - Project setup, proto, buf
2. **01-database** - SQLite schema, sqlc
3. **02-edge-core** - Webhook ingestion (parallel with 06)
4. **06-auth** - GitHub OAuth (parallel with 02)

### Phase 2: Relay System
5. **03-relay** - gRPC streaming
6. **04-home-hub** - Home service
7. **05-retry** - Exponential backoff

### Phase 3: API & UI
8. **07-api** - ConnectRPC API
9. **08-frontend** - SvelteKit UI
10. **09-embed** - Embed in Go binary

### Phase 4: Integrations
11. **10-notifications** - Telegram alerts
12. **11-mcp** - MCP server

### Phase 5: Ship
13. **12-deployment** - Docker, systemd

## Status Legend

- `NOT STARTED` - Work not begun
- `IN PROGRESS` - Currently being worked on
- `BLOCKED` - Waiting on dependency
- `DONE` - Complete and verified

## Updating Status

Edit the individual work order file and update the `Status:` line at the top.
