package relay

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"time"

	"connectrpc.com/connect"

	hooklyv1 "hooks.dx314.com/internal/api/hookly/v1"
	"hooks.dx314.com/internal/auth"
	"hooks.dx314.com/internal/db"
	"hooks.dx314.com/internal/notify"
)

const (
	heartbeatInterval = 15 * time.Second
	staleTimeout      = 60 * time.Second
)

// Handler implements the RelayService.
type Handler struct {
	tokenMgr *auth.TokenManager
	manager  *ConnectionManager
	store    *db.Store
	notifier notify.Notifier
}

// NewHandler creates a new relay handler.
func NewHandler(tokenMgr *auth.TokenManager, manager *ConnectionManager, store *db.Store, notifier notify.Notifier) *Handler {
	if notifier == nil {
		notifier = notify.NopNotifier{}
	}
	return &Handler{
		tokenMgr: tokenMgr,
		manager:  manager,
		store:    store,
		notifier: notifier,
	}
}

// Stream handles the bidirectional streaming connection from home-hub.
func (h *Handler) Stream(ctx context.Context, stream *connect.BidiStream[hooklyv1.StreamRequest, hooklyv1.StreamResponse]) error {
	// First message must be authentication
	req, err := stream.Receive()
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("expected connect message"))
	}

	connectReq := req.GetConnect()
	if connectReq == nil {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("first message must be connect request"))
	}

	// Validate bearer token
	if connectReq.Token == "" {
		return h.sendConnectError(stream, connect.CodeUnauthenticated, "TOKEN_MISSING", "no token provided - run 'hookly login' first")
	}

	token, err := h.tokenMgr.ValidateToken(ctx, connectReq.Token)
	if err != nil {
		slog.Warn("relay auth failed", "hub_id", connectReq.HubId, "error", err)
		if errors.Is(err, auth.ErrTokenNotFound) || errors.Is(err, auth.ErrInvalidToken) {
			return h.sendConnectError(stream, connect.CodeUnauthenticated, "TOKEN_INVALID", "invalid token - run 'hookly login' to re-authenticate")
		}
		if errors.Is(err, auth.ErrTokenRevoked) {
			return h.sendConnectError(stream, connect.CodeUnauthenticated, "TOKEN_REVOKED", "token has been revoked - run 'hookly login' to re-authenticate")
		}
		return h.sendConnectError(stream, connect.CodeUnauthenticated, "AUTH_FAILED", "authentication failed")
	}

	// Verify user owns the requested endpoints
	endpointIDs := connectReq.EndpointIds
	if len(endpointIDs) == 0 {
		return h.sendConnectError(stream, connect.CodeInvalidArgument, "NO_ENDPOINTS", "no endpoints specified in hookly.yaml")
	}

	for _, epID := range endpointIDs {
		ep, err := h.store.GetEndpointByID(ctx, epID)
		if err != nil {
			slog.Warn("endpoint not found", "endpoint_id", epID, "user_id", token.UserID)
			return h.sendConnectError(stream, connect.CodeNotFound, "ENDPOINT_NOT_FOUND",
				"endpoint '"+epID+"' does not exist - check your hookly.yaml or run 'hookly init' to reconfigure")
		}
		if ep.UserID != token.UserID {
			slog.Warn("endpoint ownership mismatch", "endpoint_id", epID, "user_id", token.UserID, "owner", ep.UserID)
			return h.sendConnectError(stream, connect.CodePermissionDenied, "ENDPOINT_ACCESS_DENIED",
				"you don't have access to endpoint '"+epID+"' - it belongs to another user")
		}
	}

	// Send success response
	if err := stream.Send(&hooklyv1.StreamResponse{
		Message: &hooklyv1.StreamResponse_ConnectResponse{
			ConnectResponse: &hooklyv1.ConnectResponse{
				Success: true,
			},
		},
	}); err != nil {
		return err
	}

	hubID := connectReq.HubId

	// Register connection with endpoints
	conn := h.manager.AddConnection(hubID, endpointIDs, connectReq.Capabilities)
	defer h.manager.RemoveConnection(conn)

	// Create channels for coordination
	errCh := make(chan error, 2)
	doneCh := make(chan struct{})
	defer close(doneCh)

	// Start receiver goroutine (handles ACKs and heartbeats from home-hub)
	go func() {
		for {
			select {
			case <-doneCh:
				return
			default:
			}

			msg, err := stream.Receive()
			if err != nil {
				if errors.Is(err, io.EOF) {
					errCh <- nil
				} else {
					errCh <- err
				}
				return
			}

			switch m := msg.Message.(type) {
			case *hooklyv1.StreamRequest_Ack:
				h.handleAck(ctx, conn, m.Ack)
			case *hooklyv1.StreamRequest_Heartbeat:
				h.manager.UpdateHeartbeat(hubID)
			}
		}
	}()

	// Start heartbeat sender
	heartbeatTicker := time.NewTicker(heartbeatInterval)
	defer heartbeatTicker.Stop()

	// Start stale connection checker
	staleTicker := time.NewTicker(10 * time.Second)
	defer staleTicker.Stop()

	// Main loop: send webhooks and heartbeats
	sendCh := conn.SendCh()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case err := <-errCh:
			return err

		case webhook, ok := <-sendCh:
			if !ok {
				// Replaced by a newer connection from the same hub
				return nil
			}
			if err := stream.Send(&hooklyv1.StreamResponse{
				Message: &hooklyv1.StreamResponse_Webhook{
					Webhook: webhook,
				},
			}); err != nil {
				return err
			}

		case <-heartbeatTicker.C:
			if err := stream.Send(&hooklyv1.StreamResponse{
				Message: &hooklyv1.StreamResponse_Heartbeat{
					Heartbeat: &hooklyv1.Heartbeat{
						Timestamp: time.Now().Unix(),
					},
				},
			}); err != nil {
				return err
			}

		case <-staleTicker.C:
			if h.manager.IsStale(hubID, staleTimeout) {
				slog.Warn("connection stale, closing", "hub_id", hubID)
				return connect.NewError(connect.CodeDeadlineExceeded, errors.New("connection stale"))
			}
		}
	}
}

func (h *Handler) handleAck(ctx context.Context, conn *HubConnection, ack *hooklyv1.DeliveryAck) {
	slog.Info("received delivery ack",
		"webhook_id", ack.WebhookId,
		"delivery_id", ack.DeliveryId,
		"success", ack.Success,
		"status_code", ack.StatusCode,
	)

	// Hubs that predate fan-out ack by webhook ID only
	deliveryID, err := h.store.ResolveAckDelivery(ctx, ack.WebhookId, ack.DeliveryId)
	if err != nil {
		slog.Warn("ack does not match a pending delivery", "webhook_id", ack.WebhookId, "error", err)
		return
	}
	conn.ClearInFlight(deliveryID)

	// success → delivered, permanent failure (4xx) → failed, otherwise stays
	// pending for retry after backoff. Other destinations are unaffected.
	_, err = h.store.ApplyDeliveryOutcome(ctx, deliveryID, db.DeliveryOutcome{
		Success:          ack.Success,
		PermanentFailure: ack.PermanentFailure,
		ErrorMessage:     ack.ErrorMessage,
	})
	if errors.Is(err, db.ErrDeliveryNotPending) || errors.Is(err, sql.ErrNoRows) {
		slog.Debug("ignoring ack for delivery that is not pending", "delivery_id", deliveryID)
		return
	}
	if err != nil {
		slog.Error("failed to update delivery status", "delivery_id", deliveryID, "error", err)
		return
	}

	switch {
	case ack.Success:
	case ack.PermanentFailure:
		// Send failure notification (fire and forget)
		go h.sendFailureNotification(ctx, deliveryID, ack.ErrorMessage)
	default:
		slog.Info("delivery will be retried after backoff",
			"webhook_id", ack.WebhookId,
			"delivery_id", deliveryID,
			"error", ack.ErrorMessage,
		)
	}
}

func (h *Handler) sendFailureNotification(ctx context.Context, deliveryID, errorMsg string) {
	// Get delivery with endpoint and destination info
	row, err := h.store.GetDeliveryForNotification(ctx, deliveryID)
	if err != nil {
		slog.Error("failed to get delivery for notification", "delivery_id", deliveryID, "error", err)
		return
	}

	// Check if already notified
	if row.NotificationSent != 0 {
		return
	}

	// Parse received_at time
	receivedAt, _ := time.Parse("2006-01-02 15:04:05", row.ReceivedAt)

	info := notify.WebhookInfo{
		ID:              row.WebhookID,
		EndpointID:      row.EndpointID,
		EndpointName:    row.EndpointName,
		DestinationName: row.DestinationName,
		DestinationURL:  row.DestinationUrl,
		Attempts:        int(row.Attempts),
		Error:           errorMsg,
		ReceivedAt:      receivedAt,
	}

	if err := h.notifier.NotifyDeliveryFailure(ctx, info); err != nil {
		// Log but don't fail - notification is best-effort
		return
	}

	// Mark as notified
	if err := h.store.MarkDeliveryNotificationSent(ctx, deliveryID); err != nil {
		slog.Error("failed to mark notification sent", "delivery_id", deliveryID, "error", err)
	}
}

// sendConnectError sends an error response and returns the appropriate connect error.
// The errorCode is a short machine-readable code, message is human-readable.
func (h *Handler) sendConnectError(stream *connect.BidiStream[hooklyv1.StreamRequest, hooklyv1.StreamResponse], code connect.Code, errorCode, message string) error {
	_ = stream.Send(&hooklyv1.StreamResponse{
		Message: &hooklyv1.StreamResponse_ConnectResponse{
			ConnectResponse: &hooklyv1.ConnectResponse{
				Success: false,
				Error:   errorCode + ": " + message,
			},
		},
	})
	return connect.NewError(code, errors.New(errorCode+": "+message))
}
