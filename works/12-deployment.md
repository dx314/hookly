# Work Order 12: Deployment

**Swimlane:** DevOps
**Status:** DONE
**Dependencies:** All previous work orders

---

## Objective

Create Docker images and deployment configurations.

---

## Tasks

### Edge Gateway Dockerfile
- [x] Create `deploy/edge/Dockerfile`:
  - Multi-stage build: frontend, Go builder, runtime
  - CGO enabled for SQLite
  - Alpine runtime with ca-certificates

### Home Hub Dockerfile
- [x] Create `deploy/home/Dockerfile`:
  - Multi-stage build: Go builder, runtime
  - CGO enabled for SQLite

### MCP Server Dockerfile
- [x] Create `deploy/mcp/Dockerfile`:
  - Multi-stage build: Go builder, runtime
  - ENTRYPOINT for stdio transport

### Edge Docker Compose
- [x] Create `deploy/edge/docker-compose.yml`:
  - Volume mount for data persistence
  - All environment variables configured
  - Restart policy

### Home Docker Compose
- [x] Create `deploy/home/docker-compose.yml`:
  - Environment variables for edge connection
  - Restart policy

### Environment Templates
- [x] Create `deploy/edge/.env.example`
- [x] Create `deploy/home/.env.example`

### systemd Unit Files
- [x] Create `deploy/edge/hookly-edge.service`
- [x] Create `deploy/home/hookly-home.service`

### Installation Scripts
- [x] Create `deploy/edge/install.sh`
- [x] Create `deploy/home/install.sh`

### GitHub Actions
- [x] Create `.github/workflows/build.yml`:
  - Build on push to main
  - Push to GHCR
  - Tag with version

### Makefile
- [x] Update `Makefile`:
  - docker-edge target
  - docker-home target
  - docker-mcp target
  - docker-all target

---

## Acceptance Criteria

- [x] `docker build` succeeds for all images
- [x] Edge container configuration complete
- [x] Home container configuration complete
- [x] systemd service files created
- [x] Data persists in volume (configured)
- [x] Upgrade via `docker compose pull && docker compose up -d`

---

## Notes

- CGO required for SQLite
- Alpine needs musl-dev for CGO build
- Consider multi-arch builds (amd64, arm64)

## Implementation Summary

Files created:
- `deploy/edge/Dockerfile` - Multi-stage build for edge-gateway
- `deploy/edge/docker-compose.yml` - Compose config with all env vars
- `deploy/edge/.env.example` - Environment template
- `deploy/edge/hookly-edge.service` - systemd unit
- `deploy/edge/install.sh` - Installation script
- `deploy/home/Dockerfile` - Multi-stage build for home-hub
- `deploy/home/docker-compose.yml` - Compose config
- `deploy/home/.env.example` - Environment template
- `deploy/home/hookly-home.service` - systemd unit
- `deploy/home/install.sh` - Installation script
- `deploy/mcp/Dockerfile` - Multi-stage build for MCP server
- `.github/workflows/build.yml` - CI/CD pipeline

Makefile updated with:
- `docker-edge` - Build edge image
- `docker-home` - Build home image
- `docker-mcp` - Build MCP image
- `docker-all` - Build all images
