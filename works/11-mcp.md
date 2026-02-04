# Work Order 11: MCP Server

**Swimlane:** Integration
**Status:** DONE
**Dependencies:** 07-api

---

## Objective

Implement MCP server with 9 tools for LLM integration.

---

## Tasks

### MCP Server Setup
- [x] Create `internal/mcp/server.go`:
  - Use `github.com/mark3labs/mcp-go/server` package
  - Initialize with tool definitions
  - Handle stdio transport

### Separate Binary
- [x] Create `cmd/hookly-mcp/main.go`:
  - Load config (DATABASE_PATH, ENCRYPTION_KEY)
  - Initialize database connection
  - Start MCP server on stdio

### Tool Definitions
- [x] Create `internal/mcp/tools.go`:
  - Define all 9 tools with JSON schema

### Tool: hookly_list_endpoints
```json
{
  "name": "hookly_list_endpoints",
  "description": "List all webhook endpoints",
  "inputSchema": {
    "type": "object",
    "properties": {}
  }
}
```
- [x] Implement: return all endpoints with stats

### Tool: hookly_get_endpoint
```json
{
  "name": "hookly_get_endpoint",
  "description": "Get details of a specific endpoint",
  "inputSchema": {
    "type": "object",
    "properties": {
      "endpoint_id": {"type": "string"}
    },
    "required": ["endpoint_id"]
  }
}
```
- [x] Implement: return endpoint with webhook URL

### Tool: hookly_create_endpoint
```json
{
  "name": "hookly_create_endpoint",
  "description": "Create a new webhook endpoint",
  "inputSchema": {
    "type": "object",
    "properties": {
      "name": {"type": "string"},
      "provider_type": {"type": "string", "enum": ["stripe", "github", "telegram", "generic"]},
      "signature_secret": {"type": "string"},
      "destination_url": {"type": "string"}
    },
    "required": ["name", "provider_type", "signature_secret", "destination_url"]
  }
}
```
- [x] Implement: create and return with webhook URL

### Tool: hookly_delete_endpoint
```json
{
  "name": "hookly_delete_endpoint",
  "description": "Delete a webhook endpoint",
  "inputSchema": {
    "type": "object",
    "properties": {
      "endpoint_id": {"type": "string"}
    },
    "required": ["endpoint_id"]
  }
}
```
- [x] Implement: delete endpoint and cascade webhooks

### Tool: hookly_mute_endpoint
```json
{
  "name": "hookly_mute_endpoint",
  "description": "Mute or unmute a webhook endpoint",
  "inputSchema": {
    "type": "object",
    "properties": {
      "endpoint_id": {"type": "string"},
      "muted": {"type": "boolean"}
    },
    "required": ["endpoint_id", "muted"]
  }
}
```
- [x] Implement: toggle muted flag

### Tool: hookly_list_webhooks
```json
{
  "name": "hookly_list_webhooks",
  "description": "List webhooks with optional filters",
  "inputSchema": {
    "type": "object",
    "properties": {
      "endpoint_id": {"type": "string"},
      "status": {"type": "string", "enum": ["pending", "delivered", "failed", "dead_letter"]},
      "limit": {"type": "integer", "default": 50}
    }
  }
}
```
- [x] Implement: return filtered webhook list

### Tool: hookly_get_webhook
```json
{
  "name": "hookly_get_webhook",
  "description": "Get full webhook details including payload",
  "inputSchema": {
    "type": "object",
    "properties": {
      "webhook_id": {"type": "string"}
    },
    "required": ["webhook_id"]
  }
}
```
- [x] Implement: return full webhook with payload (no redaction)

### Tool: hookly_replay_webhook
```json
{
  "name": "hookly_replay_webhook",
  "description": "Replay a webhook for re-delivery",
  "inputSchema": {
    "type": "object",
    "properties": {
      "webhook_id": {"type": "string"}
    },
    "required": ["webhook_id"]
  }
}
```
- [x] Implement: reset status to pending

### Tool: hookly_get_status
```json
{
  "name": "hookly_get_status",
  "description": "Get system status including queue depth and connection state",
  "inputSchema": {
    "type": "object",
    "properties": {}
  }
}
```
- [x] Implement: return queue stats and connection status

### Claude Desktop Config
- [x] Document MCP config:
  ```json
  {
    "mcpServers": {
      "hookly": {
        "command": "/path/to/hookly-mcp",
        "env": {
          "DATABASE_PATH": "/path/to/hookly.db",
          "ENCRYPTION_KEY": "..."
        }
      }
    }
  }
  ```

---

## Acceptance Criteria

- [x] All 9 tools implemented and working
- [x] MCP server runs on stdio
- [x] Full payload visible (no redaction)
- [x] Works with Claude Desktop
- [x] Error handling returns helpful messages
- [x] Separate binary for MCP server

---

## Notes

- MCP server is separate binary (not in edge-gateway)
- Reads from same database
- Consider connection status (requires IPC or shared state)

## Implementation Summary

Files created:
- `internal/mcp/tools.go` - Tool definitions with JSON schema
- `internal/mcp/server.go` - MCP server with all 9 tool handlers
- `cmd/hookly-mcp/main.go` - Standalone MCP binary

Dependencies added:
- `github.com/mark3labs/mcp-go` - MCP protocol implementation
- `github.com/matoous/go-nanoid/v2` - ID generation

Claude Desktop configuration example:
```json
{
  "mcpServers": {
    "hookly": {
      "command": "/path/to/hookly-mcp",
      "env": {
        "DATABASE_PATH": "/data/hookly.db",
        "ENCRYPTION_KEY": "your-32-byte-hex-key",
        "BASE_URL": "https://hooks.dx314.com"
      }
    }
  }
}
```
