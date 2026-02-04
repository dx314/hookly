# Work Order 00: Foundation

**Swimlane:** Infrastructure
**Status:** DONE
**Dependencies:** None

---

## Objective

Set up project structure, protobuf definitions, and code generation pipeline.

---

## Tasks

### Project Structure
- [x] Create Go module (`go mod init hookly`)
- [x] Create directory structure:
  ```
  hookly/
  ├── cmd/
  │   ├── edge-gateway/
  │   └── home-hub/
  ├── internal/
  │   ├── config/
  │   ├── server/
  │   ├── db/
  │   ├── webhook/
  │   ├── relay/
  │   ├── auth/
  │   ├── crypto/
  │   └── notify/
  ├── proto/
  │   └── hookly/v1/
  ├── frontend/
  ├── sql/
  └── works/
  ```

### Protobuf Setup
- [x] Create `buf.yaml` (v2 config)
- [x] Create `buf.gen.yaml` with plugins:
  - `buf.build/protocolbuffers/go`
  - `buf.build/connectrpc/go`
  - `buf.build/bufbuild/es:v2.11.0` (unified plugin for Connect-ES v2, replaces separate connectrpc/es)
- [x] Create `proto/hookly/v1/common.proto`:
  - ProviderType enum (STRIPE, GITHUB, TELEGRAM, GENERIC)
  - WebhookStatus enum (PENDING, DELIVERED, FAILED, DEAD_LETTER)
  - Endpoint message
  - Webhook message
- [x] Create `proto/hookly/v1/edge.proto`:
  - EdgeService (CRUD for endpoints, webhooks)
  - Status RPC
- [x] Create `proto/hookly/v1/relay.proto`:
  - RelayService (streaming for home-hub connection)
  - WebhookEnvelope message
  - DeliveryAck message

### Code Generation
- [x] Run `buf generate`
- [x] Verify Go code in `internal/api/hookly/v1/`
- [x] Verify TypeScript code in `frontend/src/api/hookly/v1/`

### Dependencies
- [x] Add Go dependencies:
  - `connectrpc.com/connect`
  - `google.golang.org/protobuf`
  - `github.com/go-chi/chi/v5`
  - `github.com/mattn/go-sqlite3`
  - `github.com/matoous/go-nanoid/v2`
  - `github.com/joho/godotenv`

---

## Acceptance Criteria

- [x] `buf build` succeeds with no lint errors
- [x] `buf generate` produces Go and TypeScript code
- [x] Go module compiles (`go build ./...`)
- [x] Directory structure matches spec

---

## Notes

- Reference patterns from `/home/alex/src/aura` for buf config.
- Connect-ES v2 uses unified `bufbuild/es` plugin (v2.11.0) which generates both messages and service stubs. The separate `connectrpc/es` plugin is no longer needed.
