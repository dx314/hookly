// Package mcp provides MCP server implementation for Hookly.
package mcp

import (
	"github.com/mark3labs/mcp-go/mcp"
)

// Define all 9 tools for the Hookly MCP server.
func defineTools() []mcp.Tool {
	return []mcp.Tool{
		mcp.NewTool("hookly_list_endpoints",
			mcp.WithDescription("List all webhook endpoints"),
		),
		mcp.NewTool("hookly_get_endpoint",
			mcp.WithDescription("Get details of a specific endpoint"),
			mcp.WithString("endpoint_id", mcp.Required(), mcp.Description("The endpoint ID")),
		),
		mcp.NewTool("hookly_create_endpoint",
			mcp.WithDescription("Create a new webhook endpoint"),
			mcp.WithString("name", mcp.Required(), mcp.Description("Endpoint name")),
			mcp.WithString("provider_type", mcp.Required(), mcp.Description("Provider type: stripe, github, telegram, generic, or custom")),
			mcp.WithString("signature_secret", mcp.Required(), mcp.Description("Secret for signature verification")),
			mcp.WithString("destination_url", mcp.Required(), mcp.Description("URL to forward webhooks to")),
			// Custom verification config (required when provider_type is 'custom')
			mcp.WithString("verification_method", mcp.Description("For custom provider: static, hmac_sha256, hmac_sha1, or timestamped_hmac")),
			mcp.WithString("signature_header", mcp.Description("For custom provider: header containing the signature (e.g., X-Signature)")),
			mcp.WithString("signature_prefix", mcp.Description("For custom provider: optional prefix to strip from signature (e.g., sha256=)")),
			mcp.WithString("timestamp_header", mcp.Description("For custom provider with timestamped_hmac: header containing the timestamp")),
			mcp.WithNumber("timestamp_tolerance", mcp.Description("For custom provider with timestamped_hmac: max age in seconds (default 300)")),
		),
		mcp.NewTool("hookly_delete_endpoint",
			mcp.WithDescription("Delete a webhook endpoint"),
			mcp.WithString("endpoint_id", mcp.Required(), mcp.Description("The endpoint ID to delete")),
		),
		mcp.NewTool("hookly_mute_endpoint",
			mcp.WithDescription("Mute or unmute a webhook endpoint"),
			mcp.WithString("endpoint_id", mcp.Required(), mcp.Description("The endpoint ID")),
			mcp.WithBoolean("muted", mcp.Required(), mcp.Description("Whether to mute (true) or unmute (false)")),
		),
		mcp.NewTool("hookly_list_webhooks",
			mcp.WithDescription("List webhooks with optional filters"),
			mcp.WithString("endpoint_id", mcp.Description("Filter by endpoint ID")),
			mcp.WithString("status", mcp.Description("Filter by status: pending, delivered, failed, dead_letter")),
			mcp.WithNumber("limit", mcp.Description("Maximum number of webhooks to return (default 50)")),
		),
		mcp.NewTool("hookly_get_webhook",
			mcp.WithDescription("Get full webhook details including payload"),
			mcp.WithString("webhook_id", mcp.Required(), mcp.Description("The webhook ID")),
		),
		mcp.NewTool("hookly_replay_webhook",
			mcp.WithDescription("Replay a webhook for re-delivery"),
			mcp.WithString("webhook_id", mcp.Required(), mcp.Description("The webhook ID to replay")),
		),
		mcp.NewTool("hookly_get_status",
			mcp.WithDescription("Get system status including queue depth"),
		),
	}
}
