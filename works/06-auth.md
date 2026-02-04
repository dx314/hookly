# Work Order 06: Authentication

**Swimlane:** Backend
**Status:** DONE
**Dependencies:** 00-foundation, 02-edge-core

---

## Objective

Implement GitHub OAuth for UI and API authentication.

---

## Tasks

### OAuth Flow
- [x] Create `internal/auth/github.go`:
  - `GetAuthURL(state string) string` → GitHub authorize URL
  - `ExchangeCode(code string) (*Token, error)` → Exchange code for token
  - `GetUser(token string) (*GitHubUser, error)` → Get user info
  - `CheckOrgMembership(token, org string) (bool, error)` → Check org membership

### Session Management
- [x] Create `internal/auth/session.go`:
  - Generate session token (random 32 bytes, base64)
  - Store in SQLite sessions table
  - Set HTTP-only cookie
  - Session expiry: 7 days

### Sessions Table
- [x] Add to schema:
  ```sql
  CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    username TEXT NOT NULL,
    avatar_url TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at TEXT NOT NULL
  );
  CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);
  ```

### HTTP Handlers
- [x] Create `internal/auth/handlers.go`:
  - `GET /auth/login` → Redirect to GitHub
  - `GET /auth/callback` → Handle OAuth callback
  - `POST /auth/logout` → Clear session
  - `GET /auth/me` → Get current user

### Authorization Middleware
- [x] Create `internal/auth/middleware.go`:
  - Extract session from cookie
  - Validate session (exists, not expired)
  - Attach user to context
  - Return 401 if unauthorized

### Authorization Check
- [x] Create `internal/auth/authorize.go`:
  - Check if user is authorized:
    1. If `GITHUB_ORG` set: check org membership
    2. If `GITHUB_ALLOWED_USERS` set: check username in list
    3. If neither set: allow all authenticated users
  - Cache org membership check (1 hour)

### Protected Routes
- [x] Auth middleware ready for use
- Note: Will be applied to `/api/*` routes in work order 07-api

### CSRF Protection
- [x] Generate state parameter for OAuth
- [x] Store state in cookie (short-lived, 10 minutes)
- [x] Validate state on callback

---

## Acceptance Criteria

- [x] `/auth/login` redirects to GitHub
- [x] OAuth callback creates session
- [x] Session cookie set (HTTP-only, secure based on BASE_URL)
- [x] Org membership enforced (if configured)
- [x] Username allowlist enforced (if configured)
- [x] `/auth/logout` clears session
- [x] Expired sessions rejected
- [x] CSRF state validated
- Note: API route protection (`/api/*`) will be applied in work order 07-api

---

## Files Created

- `internal/auth/github.go` - GitHub OAuth client
- `internal/auth/session.go` - Session management
- `internal/auth/handlers.go` - HTTP handlers for auth flow
- `internal/auth/middleware.go` - Auth middleware
- `internal/auth/authorize.go` - Authorization logic
- `internal/auth/session_test.go` - Session tests
- `internal/auth/authorize_test.go` - Authorization tests
- `sql/queries/sessions.sql` - Session database queries
- Updated `sql/schema.sql` - Added sessions table
- Updated `internal/db/schema.sql` - Added sessions table
- Updated `cmd/edge-gateway/main.go` - Wired up auth routes

---

## Notes

- GitHub OAuth app settings:
  - Homepage URL: `https://hooks.dx314.com`
  - Callback URL: `https://hooks.dx314.com/auth/callback`
- Scopes needed: `read:org` (for org membership check)
- Secure cookies enabled automatically when BASE_URL uses https
