package mcp

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"hooks.dx314.com/internal/db"
	"hooks.dx314.com/internal/id"
)

// Server is the MCP server for Hookly.
type Server struct {
	mcpServer     *server.MCPServer
	queries       *db.Queries
	secretManager *db.SecretManager
	baseURL       string
	userID        string
}

// NewServer creates a new Hookly MCP server.
func NewServer(queries *db.Queries, secretManager *db.SecretManager, baseURL, userID string) *Server {
	s := &Server{
		queries:       queries,
		secretManager: secretManager,
		baseURL:       baseURL,
		userID:        userID,
	}

	// Create MCP server
	s.mcpServer = server.NewMCPServer(
		"hookly",
		"1.0.0",
		server.WithToolCapabilities(false),
	)

	// Register tools
	s.registerTools()

	return s
}

// ServeStdio runs the MCP server on stdio.
func (s *Server) ServeStdio() error {
	return server.ServeStdio(s.mcpServer)
}

func (s *Server) registerTools() {
	tools := defineTools()

	handlers := map[string]server.ToolHandlerFunc{
		"hookly_list_endpoints":  s.handleListEndpoints,
		"hookly_get_endpoint":    s.handleGetEndpoint,
		"hookly_create_endpoint": s.handleCreateEndpoint,
		"hookly_delete_endpoint": s.handleDeleteEndpoint,
		"hookly_mute_endpoint":   s.handleMuteEndpoint,
		"hookly_list_webhooks":   s.handleListWebhooks,
		"hookly_get_webhook":     s.handleGetWebhook,
		"hookly_replay_webhook":  s.handleReplayWebhook,
		"hookly_get_status":      s.handleGetStatus,
	}

	for _, tool := range tools {
		if handler, ok := handlers[tool.Name]; ok {
			s.mcpServer.AddTool(tool, handler)
		}
	}
}

func (s *Server) handleListEndpoints(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	endpoints, err := s.queries.ListEndpoints(ctx, db.ListEndpointsParams{
		UserID: s.userID,
		Limit:  1000,
		Offset: 0,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list endpoints: %v", err)), nil
	}

	type endpointResult struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		ProviderType   string `json:"provider_type"`
		DestinationURL string `json:"destination_url"`
		Muted          bool   `json:"muted"`
		WebhookURL     string `json:"webhook_url"`
		CreatedAt      string `json:"created_at"`
	}

	results := make([]endpointResult, len(endpoints))
	for i, e := range endpoints {
		results[i] = endpointResult{
			ID:             e.ID,
			Name:           e.Name,
			ProviderType:   e.ProviderType,
			DestinationURL: e.DestinationUrl,
			Muted:          e.Muted != 0,
			WebhookURL:     fmt.Sprintf("%s/h/%s", s.baseURL, e.ID),
			CreatedAt:      e.CreatedAt,
		}
	}

	data, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleGetEndpoint(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	endpointID := mcp.ParseString(req, "endpoint_id", "")
	if endpointID == "" {
		return mcp.NewToolResultError("endpoint_id is required"), nil
	}

	endpoint, err := s.queries.GetEndpoint(ctx, db.GetEndpointParams{
		ID:     endpointID,
		UserID: s.userID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mcp.NewToolResultError("Endpoint not found"), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get endpoint: %v", err)), nil
	}

	result := map[string]any{
		"id":              endpoint.ID,
		"name":            endpoint.Name,
		"provider_type":   endpoint.ProviderType,
		"destination_url": endpoint.DestinationUrl,
		"muted":           endpoint.Muted != 0,
		"webhook_url":     fmt.Sprintf("%s/h/%s", s.baseURL, endpoint.ID),
		"created_at":      endpoint.CreatedAt,
		"updated_at":      endpoint.UpdatedAt,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleCreateEndpoint(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := mcp.ParseString(req, "name", "")
	providerType := mcp.ParseString(req, "provider_type", "")
	signatureSecret := mcp.ParseString(req, "signature_secret", "")
	destinationURL := mcp.ParseString(req, "destination_url", "")

	if name == "" || providerType == "" || signatureSecret == "" || destinationURL == "" {
		return mcp.NewToolResultError("name, provider_type, signature_secret, and destination_url are required"), nil
	}

	// Validate provider type
	validTypes := map[string]bool{"stripe": true, "github": true, "telegram": true, "generic": true, "custom": true}
	if !validTypes[providerType] {
		return mcp.NewToolResultError("provider_type must be one of: stripe, github, telegram, generic, custom"), nil
	}

	// Handle custom verification config
	var encryptedVerificationConfig []byte
	if providerType == "custom" {
		verificationMethod := mcp.ParseString(req, "verification_method", "")
		signatureHeader := mcp.ParseString(req, "signature_header", "")
		signaturePrefix := mcp.ParseString(req, "signature_prefix", "")
		timestampHeader := mcp.ParseString(req, "timestamp_header", "")
		timestampTolerance := mcp.ParseInt(req, "timestamp_tolerance", 300)

		if verificationMethod == "" {
			return mcp.NewToolResultError("verification_method is required for custom provider type"), nil
		}
		if signatureHeader == "" {
			return mcp.NewToolResultError("signature_header is required for custom provider type"), nil
		}

		validMethods := map[string]bool{"static": true, "hmac_sha256": true, "hmac_sha1": true, "timestamped_hmac": true}
		if !validMethods[verificationMethod] {
			return mcp.NewToolResultError("verification_method must be one of: static, hmac_sha256, hmac_sha1, timestamped_hmac"), nil
		}

		if verificationMethod == "timestamped_hmac" && timestampHeader == "" {
			return mcp.NewToolResultError("timestamp_header is required for timestamped_hmac method"), nil
		}

		// Build verification config
		verificationConfig := map[string]any{
			"method":           verificationMethod,
			"signature_header": signatureHeader,
		}
		if signaturePrefix != "" {
			verificationConfig["signature_prefix"] = signaturePrefix
		}
		if timestampHeader != "" {
			verificationConfig["timestamp_header"] = timestampHeader
		}
		if verificationMethod == "timestamped_hmac" {
			verificationConfig["timestamp_tolerance"] = timestampTolerance
		}

		configJSON, err := json.Marshal(verificationConfig)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to serialize verification config: %v", err)), nil
		}

		encryptedVerificationConfig, err = s.secretManager.EncryptSecret(string(configJSON))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to encrypt verification config: %v", err)), nil
		}
	}

	// Generate ID
	endpointID := id.NewEndpointID()

	// Encrypt secret
	encrypted, err := s.secretManager.EncryptSecret(signatureSecret)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to encrypt secret: %v", err)), nil
	}

	// Create endpoint
	endpoint, err := s.queries.CreateEndpoint(ctx, db.CreateEndpointParams{
		ID:                          endpointID,
		UserID:                      s.userID,
		Name:                        name,
		ProviderType:                providerType,
		SignatureSecretEncrypted:    encrypted,
		VerificationConfigEncrypted: encryptedVerificationConfig,
		DestinationUrl:              destinationURL,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create endpoint: %v", err)), nil
	}

	result := map[string]any{
		"id":              endpoint.ID,
		"name":            endpoint.Name,
		"provider_type":   endpoint.ProviderType,
		"destination_url": endpoint.DestinationUrl,
		"webhook_url":     fmt.Sprintf("%s/h/%s", s.baseURL, endpoint.ID),
		"created_at":      endpoint.CreatedAt,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleDeleteEndpoint(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	endpointID := mcp.ParseString(req, "endpoint_id", "")
	if endpointID == "" {
		return mcp.NewToolResultError("endpoint_id is required"), nil
	}

	// Check if endpoint exists and belongs to user
	_, err := s.queries.GetEndpoint(ctx, db.GetEndpointParams{
		ID:     endpointID,
		UserID: s.userID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mcp.NewToolResultError("Endpoint not found"), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get endpoint: %v", err)), nil
	}

	// Delete endpoint
	err = s.queries.DeleteEndpoint(ctx, db.DeleteEndpointParams{
		ID:     endpointID,
		UserID: s.userID,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to delete endpoint: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Endpoint %s deleted successfully", endpointID)), nil
}

func (s *Server) handleMuteEndpoint(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	endpointID := mcp.ParseString(req, "endpoint_id", "")
	muted := mcp.ParseBoolean(req, "muted", false)

	if endpointID == "" {
		return mcp.NewToolResultError("endpoint_id is required"), nil
	}

	var mutedInt int64
	if muted {
		mutedInt = 1
	}

	endpoint, err := s.queries.UpdateEndpoint(ctx, db.UpdateEndpointParams{
		ID:     endpointID,
		UserID: s.userID,
		Muted:  sql.NullInt64{Int64: mutedInt, Valid: true},
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mcp.NewToolResultError("Endpoint not found"), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("Failed to update endpoint: %v", err)), nil
	}

	status := "unmuted"
	if muted {
		status = "muted"
	}
	return mcp.NewToolResultText(fmt.Sprintf("Endpoint %s (%s) is now %s", endpoint.Name, endpoint.ID, status)), nil
}

func (s *Server) handleListWebhooks(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	endpointID := mcp.ParseString(req, "endpoint_id", "")
	status := mcp.ParseString(req, "status", "")
	limit := mcp.ParseInt(req, "limit", 50)

	var endpointIDVal, statusVal interface{}
	if endpointID != "" {
		endpointIDVal = endpointID
	}
	if status != "" {
		statusVal = status
	}

	webhooks, err := s.queries.ListWebhooks(ctx, db.ListWebhooksParams{
		UserID:     s.userID,
		EndpointID: endpointIDVal,
		Status:     statusVal,
		Limit:      int64(limit),
		Offset:     0,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list webhooks: %v", err)), nil
	}

	type webhookResult struct {
		ID            string `json:"id"`
		EndpointID    string `json:"endpoint_id"`
		Status        string `json:"status"`
		Attempts      int64  `json:"attempts"`
		SignatureOK   bool   `json:"signature_valid"`
		ReceivedAt    string `json:"received_at"`
		LastAttemptAt string `json:"last_attempt_at,omitempty"`
		DeliveredAt   string `json:"delivered_at,omitempty"`
		ErrorMessage  string `json:"error_message,omitempty"`
	}

	results := make([]webhookResult, len(webhooks))
	for i, w := range webhooks {
		r := webhookResult{
			ID:          w.ID,
			EndpointID:  w.EndpointID,
			Status:      w.Status,
			Attempts:    w.Attempts,
			SignatureOK: w.SignatureValid != 0,
			ReceivedAt:  w.ReceivedAt,
		}
		if w.LastAttemptAt.Valid {
			r.LastAttemptAt = w.LastAttemptAt.String
		}
		if w.DeliveredAt.Valid {
			r.DeliveredAt = w.DeliveredAt.String
		}
		if w.ErrorMessage.Valid {
			r.ErrorMessage = w.ErrorMessage.String
		}
		results[i] = r
	}

	data, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleGetWebhook(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	webhookID := mcp.ParseString(req, "webhook_id", "")
	if webhookID == "" {
		return mcp.NewToolResultError("webhook_id is required"), nil
	}

	webhook, err := s.queries.GetWebhook(ctx, db.GetWebhookParams{
		ID:     webhookID,
		UserID: s.userID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mcp.NewToolResultError("Webhook not found"), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get webhook: %v", err)), nil
	}

	// Parse headers
	var headers map[string]string
	json.Unmarshal([]byte(webhook.Headers), &headers)

	result := map[string]any{
		"id":              webhook.ID,
		"endpoint_id":     webhook.EndpointID,
		"status":          webhook.Status,
		"attempts":        webhook.Attempts,
		"signature_valid": webhook.SignatureValid != 0,
		"received_at":     webhook.ReceivedAt,
		"headers":         headers,
		"payload":         string(webhook.Payload),
		"payload_base64":  base64.StdEncoding.EncodeToString(webhook.Payload),
	}

	if webhook.LastAttemptAt.Valid {
		result["last_attempt_at"] = webhook.LastAttemptAt.String
	}
	if webhook.DeliveredAt.Valid {
		result["delivered_at"] = webhook.DeliveredAt.String
	}
	if webhook.ErrorMessage.Valid {
		result["error_message"] = webhook.ErrorMessage.String
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleReplayWebhook(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	webhookID := mcp.ParseString(req, "webhook_id", "")
	if webhookID == "" {
		return mcp.NewToolResultError("webhook_id is required"), nil
	}

	webhook, err := s.queries.ResetWebhookForReplay(ctx, db.ResetWebhookForReplayParams{
		ID:     webhookID,
		UserID: s.userID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mcp.NewToolResultError("Webhook not found"), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("Failed to replay webhook: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Webhook %s reset for replay (status: %s, attempts: %d)", webhook.ID, webhook.Status, webhook.Attempts)), nil
}

func (s *Server) handleGetStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	stats, err := s.queries.GetQueueStats(ctx, s.userID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get stats: %v", err)), nil
	}

	endpointCount, err := s.queries.CountEndpoints(ctx, s.userID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to count endpoints: %v", err)), nil
	}

	result := map[string]any{
		"queue": map[string]any{
			"pending":     stats.PendingCount,
			"failed":      stats.FailedCount,
			"dead_letter": stats.DeadLetterCount,
		},
		"endpoints_count": endpointCount,
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}
