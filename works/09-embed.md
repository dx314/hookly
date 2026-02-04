# Work Order 09: Embed Frontend

**Swimlane:** Integration
**Status:** DONE
**Dependencies:** 08-frontend

---

## Objective

Embed SvelteKit static build into Go binary.

---

## Tasks

### Go Embed Setup
- [x] Create `internal/ui/embed.go`:
  - Embed all files from `dist/` directory
  - Uses `//go:embed all:dist` directive

### Build Script
- [x] Create `Makefile`:
  - `make frontend` - builds frontend, copies to internal/ui/dist
  - `make backend` - builds Go binaries
  - `make build` - builds everything
  - `make test` - runs Go tests
  - `make dev` - runs with DEV=true for hot reload
  - `make clean` - cleans build artifacts

### Static File Server
- [x] Create `internal/ui/handler.go`:
  - Serve embedded files via fs.FS
  - Handle SPA routing (fallback to index.html)
  - Set proper content types
  - Set cache headers

### Route Registration
- [x] Mount UI handler at `/*`:
  - Catch-all for remaining routes
  - Registered after all API routes so they take precedence

### Content Type Mapping
- [x] Map file extensions to MIME types:
  - `.html` → `text/html; charset=utf-8`
  - `.js` → `application/javascript`
  - `.css` → `text/css`
  - `.svg` → `image/svg+xml`
  - `.json` → `application/json`
  - And more (png, jpg, fonts, etc.)

### Cache Headers
- [x] Hashed assets (`_app/immutable/`): `max-age=31536000, immutable`
- [x] HTML: `no-cache`
- [x] Other assets: `max-age=3600`

### Development Mode
- [x] Support `DEV=true` to serve from filesystem
- [x] Fallback to embed when `DEV` not set
- [x] DevPath parameter for local development

---

## Acceptance Criteria

- [x] Single binary serves UI
- [x] SPA routing works (deep links)
- [x] Assets cached appropriately
- [x] Binary size reasonable (18MB < 50MB target)
- [x] Dev mode serves from filesystem

---

## Files Created

- `internal/ui/embed.go` - Embed directive
- `internal/ui/handler.go` - Static file handler
- `Makefile` - Build automation

---

## Notes

- Uses `fs.Sub` to strip `dist/` prefix from embedded FS
- In dev mode, uses `os.DirFS` for filesystem access
- Chi router handles `/*` pattern for catch-all
- API routes registered first so they take precedence
