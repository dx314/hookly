# Work Order 08: Frontend

**Swimlane:** Frontend
**Status:** DONE
**Dependencies:** 07-api

---

## Objective

Build SvelteKit UI with Tailwind CSS.

---

## Tasks

### Project Setup
- [x] Create SvelteKit project in `frontend/`:
  - SvelteKit 2, Svelte 5, TypeScript
- [x] Add Tailwind CSS v4 with Vite plugin
- [x] Configure for static adapter (embed in Go)
- [x] Add ConnectRPC client setup

### ConnectRPC Client
- [x] Create `frontend/src/lib/api/client.ts`:
  - Initialize transport
  - Create EdgeService client
  - Handle auth (cookie-based)

### Layout & Navigation
- [x] Create `frontend/src/routes/+layout.svelte`:
  - Top navigation bar
  - Logo and app name
  - Navigation links
  - Login button
- [x] Navigation items:
  - Dashboard
  - Endpoints
  - Webhooks
  - Settings

### Auth Pages
- [x] Login link points to `/auth/login`
- [x] Handle unauthenticated state in API calls

### Dashboard Page (`/`)
- [x] Create `frontend/src/routes/+page.svelte`:
  - Connection status indicator (green/red)
  - Pending webhooks count
  - Failed webhooks count
  - Dead letter count
  - Quick action links

### Endpoints Pages
- [x] List page (`/endpoints`):
  - Table with: name, provider, webhook URL, status
  - Copy webhook URL button
  - Create endpoint button
  - Actions: edit, delete, mute
- [x] Create page (`/endpoints/new`):
  - Form: name, provider type, signature secret, destination URL
  - Validation
  - Cancel/Save buttons
- [x] Detail page (`/endpoints/[id]`):
  - Endpoint info
  - Webhook URL with copy
  - Recent webhooks for this endpoint
  - Mute toggle
- [x] Edit page (`/endpoints/[id]/edit`):
  - Update name, secret, destination URL
  - Provider is read-only

### Webhooks Pages
- [x] List page (`/webhooks`):
  - Table with: received, endpoint, status, attempts, signature
  - Filter by endpoint (dropdown)
  - Filter by status (dropdown)
- [x] Detail page (`/webhooks/[id]`):
  - Full webhook info
  - Headers (collapsible JSON)
  - Payload (collapsible, JSON formatted)
  - Replay button

### Settings Page (`/settings`)
- [x] Display current config (read-only)
- [x] Show base URL
- [x] Show GitHub auth status
- [x] Show Telegram notifications status

### Status Badges
- [x] Color-coded status badges:
  - pending: yellow
  - delivered: green
  - failed: red
  - dead_letter: gray

### Copy to Clipboard
- [x] Implement copy functionality for webhook URLs
- [x] Show visual feedback on success

### Build for Embedding
- [x] Configure static adapter
- [x] Output to `frontend/build/`
- [x] SPA fallback for client-side routing

---

## Acceptance Criteria

- [x] Dashboard shows queue depth and connection status
- [x] Endpoints CRUD works
- [x] Webhook URL can be copied
- [x] Webhooks list with filtering
- [x] Webhook detail shows payload
- [x] Replay button works
- [x] Responsive design (grid layout)
- [x] Dark mode support (CSS variables)
- [x] Static build embeddable in Go

---

## Files Created

- `frontend/package.json` - Dependencies and scripts
- `frontend/svelte.config.js` - SvelteKit configuration
- `frontend/vite.config.ts` - Vite with Tailwind
- `frontend/src/app.css` - Tailwind and theme CSS
- `frontend/src/app.html` - HTML template
- `frontend/src/lib/api/client.ts` - ConnectRPC client
- `frontend/src/routes/+layout.svelte` - Main layout
- `frontend/src/routes/+layout.ts` - SPA configuration
- `frontend/src/routes/+page.svelte` - Dashboard
- `frontend/src/routes/endpoints/+page.svelte` - Endpoints list
- `frontend/src/routes/endpoints/new/+page.svelte` - Create endpoint
- `frontend/src/routes/endpoints/[id]/+page.svelte` - Endpoint detail
- `frontend/src/routes/endpoints/[id]/edit/+page.svelte` - Edit endpoint
- `frontend/src/routes/webhooks/+page.svelte` - Webhooks list
- `frontend/src/routes/webhooks/[id]/+page.svelte` - Webhook detail
- `frontend/src/routes/settings/+page.svelte` - Settings

---

## Notes

- Uses Svelte 5 runes ($state, $effect, $props)
- ConnectRPC client auto-sends session cookies
- CSS uses CSS custom properties for theming
- Dark mode via prefers-color-scheme media query
- No external component library (shadcn-svelte) - custom styles for simplicity
