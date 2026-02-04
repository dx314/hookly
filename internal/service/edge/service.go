// Package edge implements the EdgeService ConnectRPC API.
package edge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	hooklyv1 "hooks.dx314.com/internal/api/hookly/v1"
	"hooks.dx314.com/internal/auth"
	"hooks.dx314.com/internal/config"
	"hooks.dx314.com/internal/db"
	"hooks.dx314.com/internal/id"
	"hooks.dx314.com/internal/relay"
)

// Service implements the EdgeService.
type Service struct {
	queries       *db.Queries
	secretManager *db.SecretManager
	connMgr       *relay.ConnectionManager
	cfg           *config.Config
}

// New creates a new EdgeService.
func New(queries *db.Queries, secretManager *db.SecretManager, connMgr *relay.ConnectionManager, cfg *config.Config) *Service {
	return &Service{
		queries:       queries,
		secretManager: secretManager,
		connMgr:       connMgr,
		cfg:           cfg,
	}
}

// generateID creates a new endpoint ID with maximum security.
func (s *Service) generateID() string {
	return id.NewEndpointID()
}

// getUserID extracts the user ID from the auth context.
// Returns NotFound error if not authenticated (prevents enumeration attacks).
func getUserID(ctx context.Context) (string, error) {
	session := auth.GetSessionFromContext(ctx)
	if session == nil {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	return session.UserID, nil
}

// CreateEndpoint creates a new webhook endpoint.
func (s *Service) CreateEndpoint(ctx context.Context, req *connect.Request[hooklyv1.CreateEndpointRequest]) (*connect.Response[hooklyv1.CreateEndpointResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	msg := req.Msg

	// Validate required fields
	if msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	if msg.DestinationUrl == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("destination_url is required"))
	}

	// Generate ID
	id := s.generateID()

	// Encrypt signature secret if provided
	var encryptedSecret []byte
	if msg.SignatureSecret != "" {
		encryptedSecret, err = s.secretManager.EncryptSecret(msg.SignatureSecret)
		if err != nil {
			slog.Error("failed to encrypt secret", "error", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to encrypt secret"))
		}
	}

	// Map provider type
	providerType := mapProviderTypeToString(msg.ProviderType)

	// Handle verification config for custom provider type
	var encryptedVerificationConfig []byte
	if providerType == "custom" {
		if msg.VerificationConfig == nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("verification_config is required for custom provider type"))
		}
		if msg.VerificationConfig.SignatureHeader == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("signature_header is required in verification_config"))
		}
		if msg.VerificationConfig.Method == hooklyv1.VerificationMethod_VERIFICATION_METHOD_UNSPECIFIED {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("method is required in verification_config"))
		}
		if msg.VerificationConfig.Method == hooklyv1.VerificationMethod_VERIFICATION_METHOD_TIMESTAMPED_HMAC && msg.VerificationConfig.TimestampHeader == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("timestamp_header is required for timestamped_hmac method"))
		}

		// Serialize verification config to JSON
		configJSON, err := json.Marshal(protoVerificationConfigToInternal(msg.VerificationConfig))
		if err != nil {
			slog.Error("failed to serialize verification config", "error", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to serialize verification config"))
		}

		// Encrypt the config
		encryptedVerificationConfig, err = s.secretManager.EncryptSecret(string(configJSON))
		if err != nil {
			slog.Error("failed to encrypt verification config", "error", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to encrypt verification config"))
		}
	}

	// Create in database
	endpoint, err := s.queries.CreateEndpoint(ctx, db.CreateEndpointParams{
		ID:                            id,
		UserID:                        userID,
		Name:                          msg.Name,
		ProviderType:                  providerType,
		SignatureSecretEncrypted:      encryptedSecret,
		VerificationConfigEncrypted:   encryptedVerificationConfig,
		DestinationUrl:                msg.DestinationUrl,
	})
	if err != nil {
		slog.Error("failed to create endpoint", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create endpoint"))
	}

	slog.Info("endpoint created", "id", id, "name", msg.Name, "user_id", userID)

	return connect.NewResponse(&hooklyv1.CreateEndpointResponse{
		Endpoint:   s.dbEndpointToProto(&endpoint),
		WebhookUrl: s.webhookURL(id),
	}), nil
}

// GetEndpoint retrieves an endpoint by ID.
func (s *Service) GetEndpoint(ctx context.Context, req *connect.Request[hooklyv1.GetEndpointRequest]) (*connect.Response[hooklyv1.GetEndpointResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	endpoint, err := s.queries.GetEndpoint(ctx, db.GetEndpointParams{
		ID:     req.Msg.Id,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("endpoint not found"))
		}
		slog.Error("failed to get endpoint", "error", err, "id", req.Msg.Id)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get endpoint"))
	}

	return connect.NewResponse(&hooklyv1.GetEndpointResponse{
		Endpoint:   s.dbEndpointToProto(&endpoint),
		WebhookUrl: s.webhookURL(endpoint.ID),
	}), nil
}

// ListEndpoints lists all endpoints with pagination.
func (s *Service) ListEndpoints(ctx context.Context, req *connect.Request[hooklyv1.ListEndpointsRequest]) (*connect.Response[hooklyv1.ListEndpointsResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Parse pagination
	pageSize := int64(50)
	offset := int64(0)

	if req.Msg.Pagination != nil {
		if req.Msg.Pagination.PageSize > 0 && req.Msg.Pagination.PageSize <= 100 {
			pageSize = int64(req.Msg.Pagination.PageSize)
		}
		if req.Msg.Pagination.PageToken != "" {
			offset, err = strconv.ParseInt(req.Msg.Pagination.PageToken, 10, 64)
			if err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid page token"))
			}
		}
	}

	endpoints, err := s.queries.ListEndpoints(ctx, db.ListEndpointsParams{
		UserID: userID,
		Limit:  pageSize + 1, // Fetch one extra to check if there's a next page
		Offset: offset,
	})
	if err != nil {
		slog.Error("failed to list endpoints", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list endpoints"))
	}

	// Get total count
	totalCount, err := s.queries.CountEndpoints(ctx, userID)
	if err != nil {
		slog.Error("failed to count endpoints", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to count endpoints"))
	}

	// Check if there's a next page
	var nextPageToken string
	if len(endpoints) > int(pageSize) {
		endpoints = endpoints[:pageSize]
		nextPageToken = strconv.FormatInt(offset+pageSize, 10)
	}

	protoEndpoints := make([]*hooklyv1.Endpoint, len(endpoints))
	for i, ep := range endpoints {
		protoEndpoints[i] = s.dbEndpointToProto(&ep)
	}

	return connect.NewResponse(&hooklyv1.ListEndpointsResponse{
		Endpoints: protoEndpoints,
		Pagination: &hooklyv1.PaginationResponse{
			NextPageToken: nextPageToken,
			TotalCount:    int32(totalCount),
		},
	}), nil
}

// UpdateEndpoint updates an existing endpoint.
func (s *Service) UpdateEndpoint(ctx context.Context, req *connect.Request[hooklyv1.UpdateEndpointRequest]) (*connect.Response[hooklyv1.UpdateEndpointResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	msg := req.Msg

	if msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	// Build update params
	params := db.UpdateEndpointParams{
		ID:     msg.Id,
		UserID: userID,
	}

	if msg.Name != nil {
		params.Name = sql.NullString{String: *msg.Name, Valid: true}
	}
	if msg.DestinationUrl != nil {
		params.DestinationUrl = sql.NullString{String: *msg.DestinationUrl, Valid: true}
	}
	if msg.Muted != nil {
		muted := int64(0)
		if *msg.Muted {
			muted = 1
		}
		params.Muted = sql.NullInt64{Int64: muted, Valid: true}
	}
	if msg.SignatureSecret != nil {
		encryptedSecret, err := s.secretManager.EncryptSecret(*msg.SignatureSecret)
		if err != nil {
			slog.Error("failed to encrypt secret", "error", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to encrypt secret"))
		}
		params.SignatureSecretEncrypted = encryptedSecret
	}

	// Handle verification config update (for custom provider type)
	if msg.VerificationConfig != nil {
		if msg.VerificationConfig.SignatureHeader == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("signature_header is required in verification_config"))
		}
		if msg.VerificationConfig.Method == hooklyv1.VerificationMethod_VERIFICATION_METHOD_UNSPECIFIED {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("method is required in verification_config"))
		}
		if msg.VerificationConfig.Method == hooklyv1.VerificationMethod_VERIFICATION_METHOD_TIMESTAMPED_HMAC && msg.VerificationConfig.TimestampHeader == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("timestamp_header is required for timestamped_hmac method"))
		}

		configJSON, err := json.Marshal(protoVerificationConfigToInternal(msg.VerificationConfig))
		if err != nil {
			slog.Error("failed to serialize verification config", "error", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to serialize verification config"))
		}

		encryptedConfig, err := s.secretManager.EncryptSecret(string(configJSON))
		if err != nil {
			slog.Error("failed to encrypt verification config", "error", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to encrypt verification config"))
		}
		params.VerificationConfigEncrypted = encryptedConfig
	}

	endpoint, err := s.queries.UpdateEndpoint(ctx, params)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("endpoint not found"))
		}
		slog.Error("failed to update endpoint", "error", err, "id", msg.Id)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update endpoint"))
	}

	slog.Info("endpoint updated", "id", msg.Id)

	return connect.NewResponse(&hooklyv1.UpdateEndpointResponse{
		Endpoint: s.dbEndpointToProto(&endpoint),
	}), nil
}

// DeleteEndpoint deletes an endpoint.
func (s *Service) DeleteEndpoint(ctx context.Context, req *connect.Request[hooklyv1.DeleteEndpointRequest]) (*connect.Response[hooklyv1.DeleteEndpointResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	// Check if endpoint exists and belongs to user
	_, err = s.queries.GetEndpoint(ctx, db.GetEndpointParams{
		ID:     req.Msg.Id,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("endpoint not found"))
		}
		slog.Error("failed to get endpoint", "error", err, "id", req.Msg.Id)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get endpoint"))
	}

	// Delete endpoint (webhooks cascade delete via FK)
	if err := s.queries.DeleteEndpoint(ctx, db.DeleteEndpointParams{
		ID:     req.Msg.Id,
		UserID: userID,
	}); err != nil {
		slog.Error("failed to delete endpoint", "error", err, "id", req.Msg.Id)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete endpoint"))
	}

	slog.Info("endpoint deleted", "id", req.Msg.Id)

	return connect.NewResponse(&hooklyv1.DeleteEndpointResponse{}), nil
}

// GetWebhook retrieves a webhook by ID.
func (s *Service) GetWebhook(ctx context.Context, req *connect.Request[hooklyv1.GetWebhookRequest]) (*connect.Response[hooklyv1.GetWebhookResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	webhook, err := s.queries.GetWebhook(ctx, db.GetWebhookParams{
		ID:     req.Msg.Id,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("webhook not found"))
		}
		slog.Error("failed to get webhook", "error", err, "id", req.Msg.Id)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get webhook"))
	}

	return connect.NewResponse(&hooklyv1.GetWebhookResponse{
		Webhook: dbWebhookToProto(&webhook),
	}), nil
}

// ListWebhooks lists webhooks with filters and pagination.
func (s *Service) ListWebhooks(ctx context.Context, req *connect.Request[hooklyv1.ListWebhooksRequest]) (*connect.Response[hooklyv1.ListWebhooksResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	msg := req.Msg

	// Parse pagination
	pageSize := int64(50)
	offset := int64(0)

	if msg.Pagination != nil {
		if msg.Pagination.PageSize > 0 && msg.Pagination.PageSize <= 100 {
			pageSize = int64(msg.Pagination.PageSize)
		}
		if msg.Pagination.PageToken != "" {
			offset, err = strconv.ParseInt(msg.Pagination.PageToken, 10, 64)
			if err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid page token"))
			}
		}
	}

	// Build filters
	var endpointID interface{}
	if msg.EndpointId != nil {
		endpointID = *msg.EndpointId
	}

	var status interface{}
	if msg.Status != nil && *msg.Status != hooklyv1.WebhookStatus_WEBHOOK_STATUS_UNSPECIFIED {
		status = mapWebhookStatusToString(*msg.Status)
	}

	webhooks, err := s.queries.ListWebhooks(ctx, db.ListWebhooksParams{
		UserID:     userID,
		EndpointID: endpointID,
		Status:     status,
		Limit:      pageSize + 1,
		Offset:     offset,
	})
	if err != nil {
		slog.Error("failed to list webhooks", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list webhooks"))
	}

	// Get total count with filters
	totalCount, err := s.queries.CountWebhooks(ctx, db.CountWebhooksParams{
		UserID:     userID,
		EndpointID: endpointID,
		Status:     status,
	})
	if err != nil {
		slog.Error("failed to count webhooks", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to count webhooks"))
	}

	// Check if there's a next page
	var nextPageToken string
	if len(webhooks) > int(pageSize) {
		webhooks = webhooks[:pageSize]
		nextPageToken = strconv.FormatInt(offset+pageSize, 10)
	}

	protoWebhooks := make([]*hooklyv1.Webhook, len(webhooks))
	for i, wh := range webhooks {
		protoWebhooks[i] = dbWebhookToProto(&wh)
	}

	return connect.NewResponse(&hooklyv1.ListWebhooksResponse{
		Webhooks: protoWebhooks,
		Pagination: &hooklyv1.PaginationResponse{
			NextPageToken: nextPageToken,
			TotalCount:    int32(totalCount),
		},
	}), nil
}

// ReplayWebhook resets a webhook for re-delivery.
func (s *Service) ReplayWebhook(ctx context.Context, req *connect.Request[hooklyv1.ReplayWebhookRequest]) (*connect.Response[hooklyv1.ReplayWebhookResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	webhook, err := s.queries.ResetWebhookForReplay(ctx, db.ResetWebhookForReplayParams{
		ID:     req.Msg.Id,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("webhook not found"))
		}
		slog.Error("failed to replay webhook", "error", err, "id", req.Msg.Id)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to replay webhook"))
	}

	slog.Info("webhook replayed", "id", req.Msg.Id)

	return connect.NewResponse(&hooklyv1.ReplayWebhookResponse{
		Webhook: dbWebhookToProto(&webhook),
	}), nil
}

// GetStatus returns system status.
func (s *Service) GetStatus(ctx context.Context, _ *connect.Request[hooklyv1.GetStatusRequest]) (*connect.Response[hooklyv1.GetStatusResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	stats, err := s.queries.GetQueueStats(ctx, userID)
	if err != nil {
		slog.Error("failed to get queue stats", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get status"))
	}

	pendingCount := int32(0)
	if stats.PendingCount.Valid {
		pendingCount = int32(stats.PendingCount.Float64)
	}
	failedCount := int32(0)
	if stats.FailedCount.Valid {
		failedCount = int32(stats.FailedCount.Float64)
	}
	deadLetterCount := int32(0)
	if stats.DeadLetterCount.Valid {
		deadLetterCount = int32(stats.DeadLetterCount.Float64)
	}

	// Get connected endpoints for this user
	connectedEndpointIDs := s.connMgr.ConnectedEndpointIDs()
	var connectedEndpoints []*hooklyv1.ConnectedEndpoint

	if len(connectedEndpointIDs) > 0 {
		// Fetch endpoint names for connected endpoints belonging to this user
		endpoints, err := s.queries.GetEndpointsByIDs(ctx, db.GetEndpointsByIDsParams{
			UserID: userID,
			Ids:    connectedEndpointIDs,
		})
		if err != nil {
			slog.Error("failed to get connected endpoints", "error", err)
			// Non-fatal, continue with empty list
		} else {
			connectedEndpoints = make([]*hooklyv1.ConnectedEndpoint, len(endpoints))
			for i, ep := range endpoints {
				connectedEndpoints[i] = &hooklyv1.ConnectedEndpoint{
					Id:   ep.ID,
					Name: ep.Name,
				}
			}
		}
	}

	status := &hooklyv1.SystemStatus{
		PendingCount:       pendingCount,
		FailedCount:        failedCount,
		DeadLetterCount:    deadLetterCount,
		ConnectedEndpoints: connectedEndpoints,
	}

	return connect.NewResponse(&hooklyv1.GetStatusResponse{
		Status: status,
	}), nil
}

// GetSettings returns system settings and user info.
func (s *Service) GetSettings(ctx context.Context, _ *connect.Request[hooklyv1.GetSettingsRequest]) (*connect.Response[hooklyv1.GetSettingsResponse], error) {
	session := auth.GetSessionFromContext(ctx)
	if session == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	return connect.NewResponse(&hooklyv1.GetSettingsResponse{
		BaseUrl:                      s.cfg.BaseURL,
		GithubAuthEnabled:            s.cfg.GitHubAuthEnabled(),
		TelegramNotificationsEnabled: s.cfg.TelegramEnabled(),
		UserId:                       session.UserID,
		Username:                     session.Username,
		AvatarUrl:                    session.AvatarURL,
	}), nil
}

// webhookURL generates the webhook URL for an endpoint.
func (s *Service) webhookURL(endpointID string) string {
	return s.cfg.BaseURL + "/h/" + endpointID
}

// Helper functions

func (s *Service) dbEndpointToProto(ep *db.Endpoint) *hooklyv1.Endpoint {
	createdAt, _ := time.Parse("2006-01-02 15:04:05", ep.CreatedAt)
	updatedAt, _ := time.Parse("2006-01-02 15:04:05", ep.UpdatedAt)

	protoEp := &hooklyv1.Endpoint{
		Id:             ep.ID,
		Name:           ep.Name,
		ProviderType:   mapStringToProviderType(ep.ProviderType),
		DestinationUrl: ep.DestinationUrl,
		Muted:          ep.Muted != 0,
		CreatedAt:      timestamppb.New(createdAt),
		UpdatedAt:      timestamppb.New(updatedAt),
	}

	// Decrypt and include verification config for custom provider type
	if ep.ProviderType == "custom" && len(ep.VerificationConfigEncrypted) > 0 {
		decrypted, err := s.secretManager.DecryptSecret(ep.VerificationConfigEncrypted)
		if err == nil {
			var cfg internalVerificationConfig
			if json.Unmarshal([]byte(decrypted), &cfg) == nil {
				protoEp.VerificationConfig = internalVerificationConfigToProto(&cfg)
			}
		}
	}

	return protoEp
}

func dbWebhookToProto(wh *db.Webhook) *hooklyv1.Webhook {
	receivedAt, _ := time.Parse("2006-01-02 15:04:05", wh.ReceivedAt)

	proto := &hooklyv1.Webhook{
		Id:             wh.ID,
		EndpointId:     wh.EndpointID,
		ReceivedAt:     timestamppb.New(receivedAt),
		Payload:        wh.Payload,
		SignatureValid: wh.SignatureValid != 0,
		Status:         mapStringToWebhookStatus(wh.Status),
		Attempts:       int32(wh.Attempts),
	}

	// Parse headers JSON
	if wh.Headers != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(wh.Headers), &headers); err == nil {
			proto.Headers = headers
		}
	}

	// Optional timestamps
	if wh.LastAttemptAt.Valid {
		t, _ := time.Parse("2006-01-02 15:04:05", wh.LastAttemptAt.String)
		proto.LastAttemptAt = timestamppb.New(t)
	}
	if wh.DeliveredAt.Valid {
		t, _ := time.Parse("2006-01-02 15:04:05", wh.DeliveredAt.String)
		proto.DeliveredAt = timestamppb.New(t)
	}
	if wh.ErrorMessage.Valid {
		proto.ErrorMessage = wh.ErrorMessage.String
	}

	return proto
}

func mapProviderTypeToString(pt hooklyv1.ProviderType) string {
	switch pt {
	case hooklyv1.ProviderType_PROVIDER_TYPE_STRIPE:
		return "stripe"
	case hooklyv1.ProviderType_PROVIDER_TYPE_GITHUB:
		return "github"
	case hooklyv1.ProviderType_PROVIDER_TYPE_TELEGRAM:
		return "telegram"
	case hooklyv1.ProviderType_PROVIDER_TYPE_GENERIC:
		return "generic"
	case hooklyv1.ProviderType_PROVIDER_TYPE_CUSTOM:
		return "custom"
	default:
		return "generic"
	}
}

func mapStringToProviderType(s string) hooklyv1.ProviderType {
	switch s {
	case "stripe":
		return hooklyv1.ProviderType_PROVIDER_TYPE_STRIPE
	case "github":
		return hooklyv1.ProviderType_PROVIDER_TYPE_GITHUB
	case "telegram":
		return hooklyv1.ProviderType_PROVIDER_TYPE_TELEGRAM
	case "generic":
		return hooklyv1.ProviderType_PROVIDER_TYPE_GENERIC
	case "custom":
		return hooklyv1.ProviderType_PROVIDER_TYPE_CUSTOM
	default:
		return hooklyv1.ProviderType_PROVIDER_TYPE_UNSPECIFIED
	}
}

func mapWebhookStatusToString(s hooklyv1.WebhookStatus) string {
	switch s {
	case hooklyv1.WebhookStatus_WEBHOOK_STATUS_PENDING:
		return "pending"
	case hooklyv1.WebhookStatus_WEBHOOK_STATUS_DELIVERED:
		return "delivered"
	case hooklyv1.WebhookStatus_WEBHOOK_STATUS_FAILED:
		return "failed"
	case hooklyv1.WebhookStatus_WEBHOOK_STATUS_DEAD_LETTER:
		return "dead_letter"
	default:
		return ""
	}
}

func mapStringToWebhookStatus(s string) hooklyv1.WebhookStatus {
	switch s {
	case "pending":
		return hooklyv1.WebhookStatus_WEBHOOK_STATUS_PENDING
	case "delivered":
		return hooklyv1.WebhookStatus_WEBHOOK_STATUS_DELIVERED
	case "failed":
		return hooklyv1.WebhookStatus_WEBHOOK_STATUS_FAILED
	case "dead_letter":
		return hooklyv1.WebhookStatus_WEBHOOK_STATUS_DEAD_LETTER
	default:
		return hooklyv1.WebhookStatus_WEBHOOK_STATUS_UNSPECIFIED
	}
}

// internalVerificationConfig matches the webhook.VerificationConfig struct for JSON serialization.
type internalVerificationConfig struct {
	Method             string `json:"method"`
	SignatureHeader    string `json:"signature_header"`
	SignaturePrefix    string `json:"signature_prefix,omitempty"`
	TimestampHeader    string `json:"timestamp_header,omitempty"`
	TimestampTolerance int64  `json:"timestamp_tolerance,omitempty"`
}

func protoVerificationConfigToInternal(cfg *hooklyv1.VerificationConfig) *internalVerificationConfig {
	if cfg == nil {
		return nil
	}
	return &internalVerificationConfig{
		Method:             mapVerificationMethodToString(cfg.Method),
		SignatureHeader:    cfg.SignatureHeader,
		SignaturePrefix:    cfg.SignaturePrefix,
		TimestampHeader:    cfg.TimestampHeader,
		TimestampTolerance: cfg.TimestampTolerance,
	}
}

func internalVerificationConfigToProto(cfg *internalVerificationConfig) *hooklyv1.VerificationConfig {
	if cfg == nil {
		return nil
	}
	return &hooklyv1.VerificationConfig{
		Method:             mapStringToVerificationMethod(cfg.Method),
		SignatureHeader:    cfg.SignatureHeader,
		SignaturePrefix:    cfg.SignaturePrefix,
		TimestampHeader:    cfg.TimestampHeader,
		TimestampTolerance: cfg.TimestampTolerance,
	}
}

func mapVerificationMethodToString(m hooklyv1.VerificationMethod) string {
	switch m {
	case hooklyv1.VerificationMethod_VERIFICATION_METHOD_STATIC:
		return "static"
	case hooklyv1.VerificationMethod_VERIFICATION_METHOD_HMAC_SHA256:
		return "hmac_sha256"
	case hooklyv1.VerificationMethod_VERIFICATION_METHOD_HMAC_SHA1:
		return "hmac_sha1"
	case hooklyv1.VerificationMethod_VERIFICATION_METHOD_TIMESTAMPED_HMAC:
		return "timestamped_hmac"
	default:
		return ""
	}
}

func mapStringToVerificationMethod(s string) hooklyv1.VerificationMethod {
	switch s {
	case "static":
		return hooklyv1.VerificationMethod_VERIFICATION_METHOD_STATIC
	case "hmac_sha256":
		return hooklyv1.VerificationMethod_VERIFICATION_METHOD_HMAC_SHA256
	case "hmac_sha1":
		return hooklyv1.VerificationMethod_VERIFICATION_METHOD_HMAC_SHA1
	case "timestamped_hmac":
		return hooklyv1.VerificationMethod_VERIFICATION_METHOD_TIMESTAMPED_HMAC
	default:
		return hooklyv1.VerificationMethod_VERIFICATION_METHOD_UNSPECIFIED
	}
}
