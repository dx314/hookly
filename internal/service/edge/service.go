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
	store         *db.Store
	secretManager *db.SecretManager
	connMgr       *relay.ConnectionManager
	cfg           *config.Config
}

// New creates a new EdgeService.
func New(store *db.Store, secretManager *db.SecretManager, connMgr *relay.ConnectionManager, cfg *config.Config) *Service {
	return &Service{
		store:         store,
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

	// destination_url is shorthand for a single destination (older clients)
	var specs []db.DestinationSpec
	for _, d := range msg.Destinations {
		specs = append(specs, db.DestinationSpec{
			Name:    d.Name,
			URL:     d.Url,
			Enabled: d.Enabled == nil || *d.Enabled,
		})
	}
	if len(specs) == 0 {
		if msg.DestinationUrl == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("at least one destination (or destination_url) is required"))
		}
		specs = []db.DestinationSpec{{Name: db.DefaultDestinationName, URL: msg.DestinationUrl, Enabled: true}}
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
	endpoint, err := s.store.CreateEndpointWithDestinations(ctx, db.CreateEndpointParams{
		ID:                          id,
		UserID:                      userID,
		Name:                        msg.Name,
		ProviderType:                providerType,
		SignatureSecretEncrypted:    encryptedSecret,
		VerificationConfigEncrypted: encryptedVerificationConfig,
	}, specs)
	if err != nil {
		if destErr := destinationError(err); destErr != nil {
			return nil, destErr
		}
		slog.Error("failed to create endpoint", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create endpoint"))
	}

	slog.Info("endpoint created", "id", id, "name", msg.Name, "user_id", userID, "destinations", len(specs))

	protoEndpoint, err := s.endpointToProto(ctx, &endpoint, false)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&hooklyv1.CreateEndpointResponse{
		Endpoint:   protoEndpoint,
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

	endpoint, err := s.store.GetEndpoint(ctx, db.GetEndpointParams{
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

	protoEndpoint, err := s.endpointToProto(ctx, &endpoint, true)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&hooklyv1.GetEndpointResponse{
		Endpoint:   protoEndpoint,
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

	endpoints, err := s.store.ListEndpoints(ctx, db.ListEndpointsParams{
		UserID: userID,
		Limit:  pageSize + 1, // Fetch one extra to check if there's a next page
		Offset: offset,
	})
	if err != nil {
		slog.Error("failed to list endpoints", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list endpoints"))
	}

	// Get total count
	totalCount, err := s.store.CountEndpoints(ctx, userID)
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
		protoEndpoints[i], err = s.endpointToProto(ctx, &ep, false)
		if err != nil {
			return nil, err
		}
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
	if msg.ProviderType != nil {
		if *msg.ProviderType == hooklyv1.ProviderType_PROVIDER_TYPE_UNSPECIFIED {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("provider_type must be set to a provider"))
		}
		if *msg.ProviderType == hooklyv1.ProviderType_PROVIDER_TYPE_CUSTOM && msg.VerificationConfig == nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("verification_config is required when switching to the custom provider type"))
		}
		params.ProviderType = sql.NullString{String: mapProviderTypeToString(*msg.ProviderType), Valid: true}
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

	// A destination_url change is applied to the primary destination
	endpoint, err := s.store.UpdateEndpointAndPrimary(ctx, params)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("endpoint not found"))
		}
		slog.Error("failed to update endpoint", "error", err, "id", msg.Id)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update endpoint"))
	}

	slog.Info("endpoint updated", "id", msg.Id)

	protoEndpoint, err := s.endpointToProto(ctx, &endpoint, false)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&hooklyv1.UpdateEndpointResponse{
		Endpoint: protoEndpoint,
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
	_, err = s.store.GetEndpoint(ctx, db.GetEndpointParams{
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
	if err := s.store.DeleteEndpoint(ctx, db.DeleteEndpointParams{
		ID:     req.Msg.Id,
		UserID: userID,
	}); err != nil {
		slog.Error("failed to delete endpoint", "error", err, "id", req.Msg.Id)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete endpoint"))
	}

	slog.Info("endpoint deleted", "id", req.Msg.Id)

	return connect.NewResponse(&hooklyv1.DeleteEndpointResponse{}), nil
}

// AddDestination adds a destination to an endpoint. It only receives webhooks
// that arrive after it was added.
func (s *Service) AddDestination(ctx context.Context, req *connect.Request[hooklyv1.AddDestinationRequest]) (*connect.Response[hooklyv1.AddDestinationResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	msg := req.Msg

	if msg.EndpointId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("endpoint_id is required"))
	}

	dest, err := s.store.AddDestination(ctx, userID, msg.EndpointId, db.DestinationSpec{
		Name:    msg.Name,
		URL:     msg.Url,
		Enabled: msg.Enabled == nil || *msg.Enabled,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("endpoint not found"))
		}
		if destErr := destinationError(err); destErr != nil {
			return nil, destErr
		}
		slog.Error("failed to add destination", "error", err, "endpoint_id", msg.EndpointId)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to add destination"))
	}

	slog.Info("destination added", "id", dest.ID, "endpoint_id", dest.EndpointID, "name", dest.Name)

	endpoint, err := s.endpointByID(ctx, userID, dest.EndpointID)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&hooklyv1.AddDestinationResponse{
		Destination: dbDestinationToProto(&dest),
		Endpoint:    endpoint,
	}), nil
}

// UpdateDestination updates a destination's name, URL or enabled flag.
func (s *Service) UpdateDestination(ctx context.Context, req *connect.Request[hooklyv1.UpdateDestinationRequest]) (*connect.Response[hooklyv1.UpdateDestinationResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	msg := req.Msg

	if msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	dest, err := s.store.UpdateDestination(ctx, userID, msg.Id, msg.Name, msg.Url, msg.Enabled)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("destination not found"))
		}
		if destErr := destinationError(err); destErr != nil {
			return nil, destErr
		}
		slog.Error("failed to update destination", "error", err, "id", msg.Id)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update destination"))
	}

	slog.Info("destination updated", "id", dest.ID, "endpoint_id", dest.EndpointID)

	endpoint, err := s.endpointByID(ctx, userID, dest.EndpointID)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&hooklyv1.UpdateDestinationResponse{
		Destination: dbDestinationToProto(&dest),
		Endpoint:    endpoint,
	}), nil
}

// RemoveDestination removes a destination and abandons its deliveries.
func (s *Service) RemoveDestination(ctx context.Context, req *connect.Request[hooklyv1.RemoveDestinationRequest]) (*connect.Response[hooklyv1.RemoveDestinationResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	endpointID, err := s.store.RemoveDestination(ctx, userID, req.Msg.Id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("destination not found"))
		}
		if destErr := destinationError(err); destErr != nil {
			return nil, destErr
		}
		slog.Error("failed to remove destination", "error", err, "id", req.Msg.Id)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to remove destination"))
	}

	slog.Info("destination removed", "id", req.Msg.Id, "endpoint_id", endpointID)

	endpoint, err := s.endpointByID(ctx, userID, endpointID)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&hooklyv1.RemoveDestinationResponse{
		Endpoint: endpoint,
	}), nil
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

	webhook, err := s.store.GetWebhook(ctx, db.GetWebhookParams{
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

	protoWebhook, err := s.webhookToProtoWithDeliveries(ctx, &webhook)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&hooklyv1.GetWebhookResponse{
		Webhook: protoWebhook,
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

	webhooks, err := s.store.ListWebhooks(ctx, db.ListWebhooksParams{
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
	totalCount, err := s.store.CountWebhooks(ctx, db.CountWebhooksParams{
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

	// Replay to one destination, or to all of them when none is given
	webhook, err := s.store.ReplayWebhook(ctx, userID, req.Msg.Id, req.Msg.GetDestinationId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("webhook or destination not found"))
		}
		if errors.Is(err, db.ErrDestinationMismatch) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		slog.Error("failed to replay webhook", "error", err, "id", req.Msg.Id)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to replay webhook"))
	}

	slog.Info("webhook replayed", "id", req.Msg.Id, "destination_id", req.Msg.GetDestinationId())

	protoWebhook, err := s.webhookToProtoWithDeliveries(ctx, &webhook)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&hooklyv1.ReplayWebhookResponse{
		Webhook: protoWebhook,
	}), nil
}

// GetStatus returns system status.
func (s *Service) GetStatus(ctx context.Context, _ *connect.Request[hooklyv1.GetStatusRequest]) (*connect.Response[hooklyv1.GetStatusResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	stats, err := s.store.GetQueueStats(ctx, userID)
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
		endpoints, err := s.store.GetEndpointsByIDs(ctx, db.GetEndpointsByIDsParams{
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

	// Try to get user's theme preference from settings
	themePreference := hooklyv1.ThemePreference_THEME_PREFERENCE_SYSTEM
	userSettings, err := s.store.GetUserSettings(ctx, session.UserID)
	if err == nil {
		themePreference = mapStringToThemePreference(userSettings.ThemePreference)
	}

	return connect.NewResponse(&hooklyv1.GetSettingsResponse{
		BaseUrl:                      s.cfg.BaseURL,
		GithubAuthEnabled:            s.cfg.GitHubAuthEnabled(),
		TelegramNotificationsEnabled: s.cfg.TelegramEnabled(),
		UserId:                       session.UserID,
		Username:                     session.Username,
		AvatarUrl:                    session.AvatarURL,
		ThemePreference:              themePreference,
		IsSuperuser:                  auth.IsSuperuser(session.Username),
	}), nil
}

// GetUserSettings returns the current user's settings.
func (s *Service) GetUserSettings(ctx context.Context, _ *connect.Request[hooklyv1.GetUserSettingsRequest]) (*connect.Response[hooklyv1.GetUserSettingsResponse], error) {
	session := auth.GetSessionFromContext(ctx)
	if session == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	settings, err := s.store.GetUserSettings(ctx, session.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Return default settings if none exist
			return connect.NewResponse(&hooklyv1.GetUserSettingsResponse{
				Settings: &hooklyv1.UserSettings{
					UserId:          session.UserID,
					Username:        session.Username,
					AvatarUrl:       session.AvatarURL,
					ThemePreference: hooklyv1.ThemePreference_THEME_PREFERENCE_SYSTEM,
					IsSuperuser:     auth.IsSuperuser(session.Username),
				},
			}), nil
		}
		slog.Error("failed to get user settings", "error", err, "user_id", session.UserID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get user settings"))
	}

	return connect.NewResponse(&hooklyv1.GetUserSettingsResponse{
		Settings: dbUserSettingsToProto(&settings, auth.IsSuperuser(session.Username)),
	}), nil
}

// UpdateUserSettings updates the current user's settings.
func (s *Service) UpdateUserSettings(ctx context.Context, req *connect.Request[hooklyv1.UpdateUserSettingsRequest]) (*connect.Response[hooklyv1.UpdateUserSettingsResponse], error) {
	session := auth.GetSessionFromContext(ctx)
	if session == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	msg := req.Msg
	var settings db.UserSetting
	var err error

	// Ensure user settings row exists (for users who logged in before migration)
	_, err = s.store.GetUserSettings(ctx, session.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		// Create the settings row
		avatarURL := sql.NullString{}
		if session.AvatarURL != "" {
			avatarURL = sql.NullString{String: session.AvatarURL, Valid: true}
		}
		_, err = s.store.UpsertUserSettings(ctx, db.UpsertUserSettingsParams{
			UserID:    session.UserID,
			Username:  session.Username,
			AvatarUrl: avatarURL,
		})
		if err != nil {
			slog.Error("failed to create user settings", "error", err, "user_id", session.UserID)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create user settings"))
		}
	} else if err != nil {
		slog.Error("failed to get user settings", "error", err, "user_id", session.UserID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get user settings"))
	}

	// Handle Telegram settings update
	if msg.TelegramBotToken != nil || msg.TelegramChatId != nil || msg.TelegramEnabled != nil {
		// Get current settings to preserve existing values
		current, err := s.store.GetUserSettings(ctx, session.UserID)
		if err != nil {
			slog.Error("failed to get user settings for update", "error", err, "user_id", session.UserID)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get user settings"))
		}

		// Build update params
		params := db.UpdateUserTelegramSettingsParams{
			UserID:                    session.UserID,
			TelegramBotTokenEncrypted: current.TelegramBotTokenEncrypted,
			TelegramChatID:            current.TelegramChatID,
			TelegramEnabled:           current.TelegramEnabled,
		}

		// Update token if provided
		if msg.TelegramBotToken != nil && *msg.TelegramBotToken != "" {
			encryptedToken, err := s.secretManager.EncryptSecret(*msg.TelegramBotToken)
			if err != nil {
				slog.Error("failed to encrypt telegram token", "error", err)
				return nil, connect.NewError(connect.CodeInternal, errors.New("failed to encrypt telegram token"))
			}
			params.TelegramBotTokenEncrypted = encryptedToken
		}

		// Update chat ID if provided
		if msg.TelegramChatId != nil {
			params.TelegramChatID = sql.NullString{String: *msg.TelegramChatId, Valid: *msg.TelegramChatId != ""}
		}

		// Update enabled flag if provided
		if msg.TelegramEnabled != nil {
			if *msg.TelegramEnabled {
				params.TelegramEnabled = 1
			} else {
				params.TelegramEnabled = 0
			}
		}

		settings, err = s.store.UpdateUserTelegramSettings(ctx, params)
		if err != nil {
			slog.Error("failed to update telegram settings", "error", err, "user_id", session.UserID)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update telegram settings"))
		}

		slog.Info("user telegram settings updated", "user_id", session.UserID)
	}

	// Handle theme preference update
	if msg.ThemePreference != nil && *msg.ThemePreference != hooklyv1.ThemePreference_THEME_PREFERENCE_UNSPECIFIED {
		themeStr := mapThemePreferenceToString(*msg.ThemePreference)
		settings, err = s.store.UpdateUserTheme(ctx, db.UpdateUserThemeParams{
			UserID:          session.UserID,
			ThemePreference: themeStr,
		})
		if err != nil {
			slog.Error("failed to update theme preference", "error", err, "user_id", session.UserID)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update theme preference"))
		}

		slog.Info("user theme updated", "user_id", session.UserID, "theme", themeStr)
	}

	// If no updates were made, fetch current settings
	if settings.UserID == "" {
		settings, err = s.store.GetUserSettings(ctx, session.UserID)
		if err != nil {
			slog.Error("failed to get user settings", "error", err, "user_id", session.UserID)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get user settings"))
		}
	}

	return connect.NewResponse(&hooklyv1.UpdateUserSettingsResponse{
		Settings: dbUserSettingsToProto(&settings, auth.IsSuperuser(session.Username)),
	}), nil
}

// GetSystemSettings returns system-wide settings (superuser only).
func (s *Service) GetSystemSettings(ctx context.Context, _ *connect.Request[hooklyv1.GetSystemSettingsRequest]) (*connect.Response[hooklyv1.GetSystemSettingsResponse], error) {
	session := auth.GetSessionFromContext(ctx)
	if session == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	if !auth.IsSuperuser(session.Username) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser access required"))
	}

	// Get stats
	totalUsers, err := s.store.CountUsers(ctx)
	if err != nil {
		slog.Error("failed to count users", "error", err)
		totalUsers = 0
	}

	totalEndpoints, err := s.store.CountAllEndpoints(ctx)
	if err != nil {
		slog.Error("failed to count endpoints", "error", err)
		totalEndpoints = 0
	}

	return connect.NewResponse(&hooklyv1.GetSystemSettingsResponse{
		Settings: &hooklyv1.SystemSettings{
			BaseUrl:               s.cfg.BaseURL,
			GithubOrg:             s.cfg.GitHubOrg,
			GithubAllowedUsers:    s.cfg.GitHubAllowedUsers,
			SystemTelegramEnabled: s.cfg.TelegramEnabled(),
			TotalUsers:            int32(totalUsers),
			TotalEndpoints:        int32(totalEndpoints),
		},
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

// destinationError maps destination validation errors to ConnectRPC errors.
// Returns nil for errors that are not the caller's fault.
func destinationError(err error) *connect.Error {
	switch {
	case errors.Is(err, db.ErrInvalidDestination):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, db.ErrDuplicateDestinationName):
		return connect.NewError(connect.CodeAlreadyExists, err)
	case errors.Is(err, db.ErrLastDestination), errors.Is(err, db.ErrLastEnabledDestination):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	default:
		return nil
	}
}

// endpointByID loads an endpoint with its destinations.
func (s *Service) endpointByID(ctx context.Context, userID, id string) (*hooklyv1.Endpoint, error) {
	endpoint, err := s.store.GetEndpoint(ctx, db.GetEndpointParams{ID: id, UserID: userID})
	if err != nil {
		slog.Error("failed to get endpoint", "error", err, "id", id)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get endpoint"))
	}
	return s.endpointToProto(ctx, &endpoint, false)
}

// endpointToProto converts an endpoint and loads its destinations, optionally
// with per-destination delivery stats.
func (s *Service) endpointToProto(ctx context.Context, ep *db.Endpoint, withStats bool) (*hooklyv1.Endpoint, error) {
	dests, err := s.store.ListDestinationsByEndpoint(ctx, ep.ID)
	if err != nil {
		slog.Error("failed to list destinations", "error", err, "endpoint_id", ep.ID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list destinations"))
	}

	stats := make(map[string]*hooklyv1.DestinationStats)
	if withStats {
		rows, err := s.store.GetDestinationDeliveryStats(ctx, ep.ID)
		if err != nil {
			slog.Error("failed to get destination stats", "error", err, "endpoint_id", ep.ID)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get destination stats"))
		}
		for _, row := range rows {
			st := &hooklyv1.DestinationStats{
				PendingCount:    int32(row.PendingCount),
				DeliveredCount:  int32(row.DeliveredCount),
				FailedCount:     int32(row.FailedCount),
				DeadLetterCount: int32(row.DeadLetterCount),
			}
			if row.LastDeliveredAt != "" {
				t, _ := time.Parse("2006-01-02 15:04:05", row.LastDeliveredAt)
				st.LastDeliveredAt = timestamppb.New(t)
			}
			if row.PendingCount+row.FailedCount+row.DeadLetterCount > 0 {
				lastError, err := s.store.GetDestinationLastError(ctx, row.DestinationID)
				if err != nil && !errors.Is(err, sql.ErrNoRows) {
					slog.Error("failed to get destination last error", "error", err, "destination_id", row.DestinationID)
				}
				st.LastError = lastError
			}
			stats[row.DestinationID] = st
		}
	}

	proto := s.dbEndpointToProto(ep)
	proto.Destinations = make([]*hooklyv1.Destination, len(dests))
	for i, d := range dests {
		proto.Destinations[i] = dbDestinationToProto(&d)
		if withStats {
			proto.Destinations[i].Stats = stats[d.ID]
			if proto.Destinations[i].Stats == nil {
				proto.Destinations[i].Stats = &hooklyv1.DestinationStats{}
			}
		}
	}
	// destination_url is the primary destination for older clients
	if len(dests) > 0 {
		proto.DestinationUrl = dests[0].Url
	}

	return proto, nil
}

func dbDestinationToProto(d *db.Destination) *hooklyv1.Destination {
	createdAt, _ := time.Parse("2006-01-02 15:04:05", d.CreatedAt)
	updatedAt, _ := time.Parse("2006-01-02 15:04:05", d.UpdatedAt)

	return &hooklyv1.Destination{
		Id:         d.ID,
		EndpointId: d.EndpointID,
		Name:       d.Name,
		Url:        d.Url,
		Enabled:    d.Enabled != 0,
		Position:   int32(d.Position),
		CreatedAt:  timestamppb.New(createdAt),
		UpdatedAt:  timestamppb.New(updatedAt),
	}
}

// webhookToProtoWithDeliveries converts a webhook and loads its per-destination deliveries.
func (s *Service) webhookToProtoWithDeliveries(ctx context.Context, wh *db.Webhook) (*hooklyv1.Webhook, error) {
	rows, err := s.store.ListDeliveriesByWebhook(ctx, wh.ID)
	if err != nil {
		slog.Error("failed to list deliveries", "error", err, "webhook_id", wh.ID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list deliveries"))
	}

	proto := dbWebhookToProto(wh)
	proto.Deliveries = make([]*hooklyv1.Delivery, len(rows))
	for i, row := range rows {
		delivery := &hooklyv1.Delivery{
			Id:              row.ID,
			WebhookId:       row.WebhookID,
			DestinationId:   row.DestinationID,
			DestinationName: row.DestinationName,
			DestinationUrl:  row.DestinationUrl,
			Status:          mapStringToWebhookStatus(row.Status),
			Attempts:        int32(row.Attempts),
		}
		if row.LastAttemptAt.Valid {
			t, _ := time.Parse("2006-01-02 15:04:05", row.LastAttemptAt.String)
			delivery.LastAttemptAt = timestamppb.New(t)
		}
		if row.DeliveredAt.Valid {
			t, _ := time.Parse("2006-01-02 15:04:05", row.DeliveredAt.String)
			delivery.DeliveredAt = timestamppb.New(t)
		}
		if row.ErrorMessage.Valid {
			delivery.ErrorMessage = row.ErrorMessage.String
		}
		proto.Deliveries[i] = delivery
	}

	return proto, nil
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

func mapThemePreferenceToString(t hooklyv1.ThemePreference) string {
	switch t {
	case hooklyv1.ThemePreference_THEME_PREFERENCE_SYSTEM:
		return "system"
	case hooklyv1.ThemePreference_THEME_PREFERENCE_LIGHT:
		return "light"
	case hooklyv1.ThemePreference_THEME_PREFERENCE_DARK:
		return "dark"
	case hooklyv1.ThemePreference_THEME_PREFERENCE_PLACID_BLUE_LIGHT:
		return "placid-blue-light"
	case hooklyv1.ThemePreference_THEME_PREFERENCE_PLACID_BLUE_DARK:
		return "placid-blue-dark"
	default:
		return "system"
	}
}

func mapStringToThemePreference(s string) hooklyv1.ThemePreference {
	switch s {
	case "system":
		return hooklyv1.ThemePreference_THEME_PREFERENCE_SYSTEM
	case "light":
		return hooklyv1.ThemePreference_THEME_PREFERENCE_LIGHT
	case "dark":
		return hooklyv1.ThemePreference_THEME_PREFERENCE_DARK
	case "placid-blue-light":
		return hooklyv1.ThemePreference_THEME_PREFERENCE_PLACID_BLUE_LIGHT
	case "placid-blue-dark":
		return hooklyv1.ThemePreference_THEME_PREFERENCE_PLACID_BLUE_DARK
	default:
		return hooklyv1.ThemePreference_THEME_PREFERENCE_SYSTEM
	}
}

func dbUserSettingsToProto(s *db.UserSetting, isSuperuser bool) *hooklyv1.UserSettings {
	createdAt, _ := time.Parse("2006-01-02 15:04:05", s.CreatedAt)
	updatedAt, _ := time.Parse("2006-01-02 15:04:05", s.UpdatedAt)
	lastLoginAt, _ := time.Parse("2006-01-02 15:04:05", s.LastLoginAt)

	return &hooklyv1.UserSettings{
		UserId:             s.UserID,
		Username:           s.Username,
		GithubName:         s.GithubName.String,
		GithubEmail:        s.GithubEmail.String,
		GithubProfileUrl:   s.GithubProfileUrl.String,
		AvatarUrl:          s.AvatarUrl.String,
		TelegramConfigured: len(s.TelegramBotTokenEncrypted) > 0,
		TelegramChatId:     s.TelegramChatID.String,
		TelegramEnabled:    s.TelegramEnabled != 0,
		ThemePreference:    mapStringToThemePreference(s.ThemePreference),
		IsSuperuser:        isSuperuser,
		CreatedAt:          timestamppb.New(createdAt),
		UpdatedAt:          timestamppb.New(updatedAt),
		LastLoginAt:        timestamppb.New(lastLoginAt),
	}
}
