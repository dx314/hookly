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
	store         *db.Store
	secretManager *db.SecretManager
	baseURL       string
	userID        string
}

// NewServer creates a new Hookly MCP server.
func NewServer(store *db.Store, secretManager *db.SecretManager, baseURL, userID string) *Server {
	s := &Server{
		store:         store,
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
		"hookly_list_endpoints":     s.handleListEndpoints,
		"hookly_get_endpoint":       s.handleGetEndpoint,
		"hookly_create_endpoint":    s.handleCreateEndpoint,
		"hookly_delete_endpoint":    s.handleDeleteEndpoint,
		"hookly_mute_endpoint":      s.handleMuteEndpoint,
		"hookly_add_destination":    s.handleAddDestination,
		"hookly_update_destination": s.handleUpdateDestination,
		"hookly_remove_destination": s.handleRemoveDestination,
		"hookly_list_webhooks":      s.handleListWebhooks,
		"hookly_get_webhook":        s.handleGetWebhook,
		"hookly_replay_webhook":     s.handleReplayWebhook,
		"hookly_get_status":         s.handleGetStatus,
	}

	for _, tool := range tools {
		if handler, ok := handlers[tool.Name]; ok {
			s.mcpServer.AddTool(tool, handler)
		}
	}
}

func (s *Server) handleListEndpoints(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	endpoints, err := s.store.ListEndpoints(ctx, db.ListEndpointsParams{
		UserID: s.userID,
		Limit:  1000,
		Offset: 0,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list endpoints: %v", err)), nil
	}

	type endpointResult struct {
		ID             string              `json:"id"`
		Name           string              `json:"name"`
		ProviderType   string              `json:"provider_type"`
		DestinationURL string              `json:"destination_url"` // primary destination
		Destinations   []destinationResult `json:"destinations"`
		Muted          bool                `json:"muted"`
		WebhookURL     string              `json:"webhook_url"`
		CreatedAt      string              `json:"created_at"`
	}

	results := make([]endpointResult, len(endpoints))
	for i, e := range endpoints {
		destinations, err := s.destinations(ctx, e.ID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list destinations: %v", err)), nil
		}
		results[i] = endpointResult{
			ID:             e.ID,
			Name:           e.Name,
			ProviderType:   e.ProviderType,
			DestinationURL: primaryURL(destinations, e.DestinationUrl),
			Destinations:   destinations,
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

	endpoint, err := s.store.GetEndpoint(ctx, db.GetEndpointParams{
		ID:     endpointID,
		UserID: s.userID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mcp.NewToolResultError("Endpoint not found"), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get endpoint: %v", err)), nil
	}

	destinations, err := s.destinations(ctx, endpoint.ID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list destinations: %v", err)), nil
	}

	result := map[string]any{
		"id":              endpoint.ID,
		"name":            endpoint.Name,
		"provider_type":   endpoint.ProviderType,
		"destination_url": primaryURL(destinations, endpoint.DestinationUrl),
		"destinations":    destinations,
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

	if name == "" || providerType == "" || signatureSecret == "" {
		return mcp.NewToolResultError("name, provider_type, and signature_secret are required"), nil
	}

	// destination_url is shorthand for a single destination named "default"
	specs, err := parseDestinationSpecs(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if len(specs) == 0 {
		if destinationURL == "" {
			return mcp.NewToolResultError("destination_url or destinations is required"), nil
		}
		specs = []db.DestinationSpec{{Name: db.DefaultDestinationName, URL: destinationURL, Enabled: true}}
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
	endpoint, err := s.store.CreateEndpointWithDestinations(ctx, db.CreateEndpointParams{
		ID:                          endpointID,
		UserID:                      s.userID,
		Name:                        name,
		ProviderType:                providerType,
		SignatureSecretEncrypted:    encrypted,
		VerificationConfigEncrypted: encryptedVerificationConfig,
	}, specs)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create endpoint: %v", err)), nil
	}

	destinations, err := s.destinations(ctx, endpoint.ID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list destinations: %v", err)), nil
	}

	result := map[string]any{
		"id":              endpoint.ID,
		"name":            endpoint.Name,
		"provider_type":   endpoint.ProviderType,
		"destination_url": endpoint.DestinationUrl,
		"destinations":    destinations,
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
	_, err := s.store.GetEndpoint(ctx, db.GetEndpointParams{
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
	err = s.store.DeleteEndpoint(ctx, db.DeleteEndpointParams{
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

	endpoint, err := s.store.UpdateEndpoint(ctx, db.UpdateEndpointParams{
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

	webhooks, err := s.store.ListWebhooks(ctx, db.ListWebhooksParams{
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

	webhook, err := s.store.GetWebhook(ctx, db.GetWebhookParams{
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

	// Per-destination delivery state (the webhook's status above is derived from these)
	deliveries, err := s.store.ListDeliveriesByWebhook(ctx, webhookID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list deliveries: %v", err)), nil
	}
	deliveryResults := make([]map[string]any, len(deliveries))
	for i, d := range deliveries {
		dr := map[string]any{
			"destination_id":   d.DestinationID,
			"destination_name": d.DestinationName,
			"destination_url":  d.DestinationUrl,
			"status":           d.Status,
			"attempts":         d.Attempts,
		}
		if d.LastAttemptAt.Valid {
			dr["last_attempt_at"] = d.LastAttemptAt.String
		}
		if d.DeliveredAt.Valid {
			dr["delivered_at"] = d.DeliveredAt.String
		}
		if d.ErrorMessage.Valid {
			dr["error_message"] = d.ErrorMessage.String
		}
		deliveryResults[i] = dr
	}
	result["deliveries"] = deliveryResults

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleReplayWebhook(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	webhookID := mcp.ParseString(req, "webhook_id", "")
	if webhookID == "" {
		return mcp.NewToolResultError("webhook_id is required"), nil
	}

	// Optional: replay to a single destination instead of all of them
	destinationID := mcp.ParseString(req, "destination_id", "")

	webhook, err := s.store.ReplayWebhook(ctx, s.userID, webhookID, destinationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mcp.NewToolResultError("Webhook or destination not found"), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("Failed to replay webhook: %v", err)), nil
	}

	target := "all destinations"
	if destinationID != "" {
		target = "destination " + destinationID
	}
	return mcp.NewToolResultText(fmt.Sprintf("Webhook %s reset for replay to %s (status: %s, attempts: %d)", webhook.ID, target, webhook.Status, webhook.Attempts)), nil
}

func (s *Server) handleAddDestination(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	endpointID := mcp.ParseString(req, "endpoint_id", "")
	name := mcp.ParseString(req, "name", "")
	url := mcp.ParseString(req, "url", "")

	if endpointID == "" || name == "" || url == "" {
		return mcp.NewToolResultError("endpoint_id, name, and url are required"), nil
	}

	dest, err := s.store.AddDestination(ctx, s.userID, endpointID, db.DestinationSpec{
		Name:    name,
		URL:     url,
		Enabled: mcp.ParseBoolean(req, "enabled", true),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mcp.NewToolResultError("Endpoint not found"), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("Failed to add destination: %v", err)), nil
	}

	data, _ := json.MarshalIndent(toDestinationResult(dest), "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleUpdateDestination(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	destinationID := mcp.ParseString(req, "destination_id", "")
	if destinationID == "" {
		return mcp.NewToolResultError("destination_id is required"), nil
	}

	// Only fields that are present are changed
	args := req.GetArguments()
	var name, url *string
	var enabled *bool
	if _, ok := args["name"]; ok {
		v := mcp.ParseString(req, "name", "")
		name = &v
	}
	if _, ok := args["url"]; ok {
		v := mcp.ParseString(req, "url", "")
		url = &v
	}
	if _, ok := args["enabled"]; ok {
		v := mcp.ParseBoolean(req, "enabled", true)
		enabled = &v
	}

	dest, err := s.store.UpdateDestination(ctx, s.userID, destinationID, name, url, enabled)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mcp.NewToolResultError("Destination not found"), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("Failed to update destination: %v", err)), nil
	}

	data, _ := json.MarshalIndent(toDestinationResult(dest), "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleRemoveDestination(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	destinationID := mcp.ParseString(req, "destination_id", "")
	if destinationID == "" {
		return mcp.NewToolResultError("destination_id is required"), nil
	}

	endpointID, err := s.store.RemoveDestination(ctx, s.userID, destinationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mcp.NewToolResultError("Destination not found"), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("Failed to remove destination: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Destination %s removed from endpoint %s (its pending deliveries were abandoned)", destinationID, endpointID)), nil
}

// destinationResult is the JSON shape of a destination in tool results.
type destinationResult struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
}

func toDestinationResult(d db.Destination) destinationResult {
	return destinationResult{ID: d.ID, Name: d.Name, URL: d.Url, Enabled: d.Enabled != 0}
}

// destinations returns an endpoint's destinations, primary first.
func (s *Server) destinations(ctx context.Context, endpointID string) ([]destinationResult, error) {
	dests, err := s.store.ListDestinationsByEndpoint(ctx, endpointID)
	if err != nil {
		return nil, err
	}
	results := make([]destinationResult, len(dests))
	for i, d := range dests {
		results[i] = toDestinationResult(d)
	}
	return results, nil
}

func primaryURL(destinations []destinationResult, fallback string) string {
	if len(destinations) > 0 {
		return destinations[0].URL
	}
	return fallback
}

// parseDestinationSpecs reads the optional "destinations" array of {name, url, enabled}.
func parseDestinationSpecs(req mcp.CallToolRequest) ([]db.DestinationSpec, error) {
	raw, ok := req.GetArguments()["destinations"]
	if !ok || raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("destinations must be an array of {name, url} objects")
	}

	specs := make([]db.DestinationSpec, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("destinations must be an array of {name, url} objects")
		}
		spec := db.DestinationSpec{Enabled: true}
		spec.Name, _ = obj["name"].(string)
		spec.URL, _ = obj["url"].(string)
		if enabled, ok := obj["enabled"].(bool); ok {
			spec.Enabled = enabled
		}
		if spec.Name == "" || spec.URL == "" {
			return nil, fmt.Errorf("each destination needs a name and a url")
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

func (s *Server) handleGetStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	stats, err := s.store.GetQueueStats(ctx, s.userID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get stats: %v", err)), nil
	}

	endpointCount, err := s.store.CountEndpoints(ctx, s.userID)
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
