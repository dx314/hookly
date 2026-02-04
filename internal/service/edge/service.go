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
	gonanoid "github.com/matoous/go-nanoid/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	hooklyv1 "hookly/internal/api/hookly/v1"
	"hookly/internal/config"
	"hookly/internal/db"
	"hookly/internal/relay"
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

// generateID creates a new nanoid.
func (s *Service) generateID() string {
	id, _ := gonanoid.New()
	return id
}

// CreateEndpoint creates a new webhook endpoint.
func (s *Service) CreateEndpoint(ctx context.Context, req *connect.Request[hooklyv1.CreateEndpointRequest]) (*connect.Response[hooklyv1.CreateEndpointResponse], error) {
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
		var err error
		encryptedSecret, err = s.secretManager.EncryptSecret(msg.SignatureSecret)
		if err != nil {
			slog.Error("failed to encrypt secret", "error", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to encrypt secret"))
		}
	}

	// Map provider type
	providerType := mapProviderTypeToString(msg.ProviderType)

	// Create in database
	endpoint, err := s.queries.CreateEndpoint(ctx, db.CreateEndpointParams{
		ID:                       id,
		Name:                     msg.Name,
		ProviderType:             providerType,
		SignatureSecretEncrypted: encryptedSecret,
		DestinationUrl:           msg.DestinationUrl,
	})
	if err != nil {
		slog.Error("failed to create endpoint", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create endpoint"))
	}

	slog.Info("endpoint created", "id", id, "name", msg.Name)

	return connect.NewResponse(&hooklyv1.CreateEndpointResponse{
		Endpoint:   dbEndpointToProto(&endpoint),
		WebhookUrl: s.webhookURL(id),
	}), nil
}

// GetEndpoint retrieves an endpoint by ID.
func (s *Service) GetEndpoint(ctx context.Context, req *connect.Request[hooklyv1.GetEndpointRequest]) (*connect.Response[hooklyv1.GetEndpointResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	endpoint, err := s.queries.GetEndpoint(ctx, req.Msg.Id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("endpoint not found"))
		}
		slog.Error("failed to get endpoint", "error", err, "id", req.Msg.Id)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get endpoint"))
	}

	return connect.NewResponse(&hooklyv1.GetEndpointResponse{
		Endpoint:   dbEndpointToProto(&endpoint),
		WebhookUrl: s.webhookURL(endpoint.ID),
	}), nil
}

// ListEndpoints lists all endpoints with pagination.
func (s *Service) ListEndpoints(ctx context.Context, req *connect.Request[hooklyv1.ListEndpointsRequest]) (*connect.Response[hooklyv1.ListEndpointsResponse], error) {
	// Parse pagination
	pageSize := int64(50)
	offset := int64(0)

	if req.Msg.Pagination != nil {
		if req.Msg.Pagination.PageSize > 0 && req.Msg.Pagination.PageSize <= 100 {
			pageSize = int64(req.Msg.Pagination.PageSize)
		}
		if req.Msg.Pagination.PageToken != "" {
			var err error
			offset, err = strconv.ParseInt(req.Msg.Pagination.PageToken, 10, 64)
			if err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid page token"))
			}
		}
	}

	endpoints, err := s.queries.ListEndpoints(ctx, db.ListEndpointsParams{
		Limit:  pageSize + 1, // Fetch one extra to check if there's a next page
		Offset: offset,
	})
	if err != nil {
		slog.Error("failed to list endpoints", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list endpoints"))
	}

	// Get total count
	totalCount, err := s.queries.CountEndpoints(ctx)
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
		protoEndpoints[i] = dbEndpointToProto(&ep)
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
	msg := req.Msg

	if msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	// Build update params
	params := db.UpdateEndpointParams{
		ID: msg.Id,
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
		Endpoint: dbEndpointToProto(&endpoint),
	}), nil
}

// DeleteEndpoint deletes an endpoint.
func (s *Service) DeleteEndpoint(ctx context.Context, req *connect.Request[hooklyv1.DeleteEndpointRequest]) (*connect.Response[hooklyv1.DeleteEndpointResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	// Check if endpoint exists
	_, err := s.queries.GetEndpoint(ctx, req.Msg.Id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("endpoint not found"))
		}
		slog.Error("failed to get endpoint", "error", err, "id", req.Msg.Id)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get endpoint"))
	}

	// Delete endpoint (webhooks cascade delete via FK)
	if err := s.queries.DeleteEndpoint(ctx, req.Msg.Id); err != nil {
		slog.Error("failed to delete endpoint", "error", err, "id", req.Msg.Id)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete endpoint"))
	}

	slog.Info("endpoint deleted", "id", req.Msg.Id)

	return connect.NewResponse(&hooklyv1.DeleteEndpointResponse{}), nil
}

// GetWebhook retrieves a webhook by ID.
func (s *Service) GetWebhook(ctx context.Context, req *connect.Request[hooklyv1.GetWebhookRequest]) (*connect.Response[hooklyv1.GetWebhookResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	webhook, err := s.queries.GetWebhook(ctx, req.Msg.Id)
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
	msg := req.Msg

	// Parse pagination
	pageSize := int64(50)
	offset := int64(0)

	if msg.Pagination != nil {
		if msg.Pagination.PageSize > 0 && msg.Pagination.PageSize <= 100 {
			pageSize = int64(msg.Pagination.PageSize)
		}
		if msg.Pagination.PageToken != "" {
			var err error
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
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	webhook, err := s.queries.ResetWebhookForReplay(ctx, req.Msg.Id)
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
	stats, err := s.queries.GetQueueStats(ctx)
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

	status := &hooklyv1.SystemStatus{
		PendingCount:    pendingCount,
		FailedCount:     failedCount,
		DeadLetterCount: deadLetterCount,
		HomeHubConnected: s.connMgr.IsConnected(),
	}

	if s.connMgr.IsConnected() {
		lastHeartbeat := s.connMgr.LastHeartbeat()
		status.LastHomeHubHeartbeat = timestamppb.New(lastHeartbeat)
	}

	return connect.NewResponse(&hooklyv1.GetStatusResponse{
		Status: status,
	}), nil
}

// GetSettings returns system settings (with secrets redacted).
func (s *Service) GetSettings(ctx context.Context, _ *connect.Request[hooklyv1.GetSettingsRequest]) (*connect.Response[hooklyv1.GetSettingsResponse], error) {
	return connect.NewResponse(&hooklyv1.GetSettingsResponse{
		BaseUrl:                     s.cfg.BaseURL,
		GithubAuthEnabled:           s.cfg.GitHubAuthEnabled(),
		TelegramNotificationsEnabled: s.cfg.TelegramEnabled(),
	}), nil
}

// webhookURL generates the webhook URL for an endpoint.
func (s *Service) webhookURL(endpointID string) string {
	return s.cfg.BaseURL + "/h/" + endpointID
}

// Helper functions

func dbEndpointToProto(ep *db.Endpoint) *hooklyv1.Endpoint {
	createdAt, _ := time.Parse("2006-01-02 15:04:05", ep.CreatedAt)
	updatedAt, _ := time.Parse("2006-01-02 15:04:05", ep.UpdatedAt)

	return &hooklyv1.Endpoint{
		Id:             ep.ID,
		Name:           ep.Name,
		ProviderType:   mapStringToProviderType(ep.ProviderType),
		DestinationUrl: ep.DestinationUrl,
		Muted:          ep.Muted != 0,
		CreatedAt:      timestamppb.New(createdAt),
		UpdatedAt:      timestamppb.New(updatedAt),
	}
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
