package relay

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"time"

	"connectrpc.com/connect"

	hooklyv1 "hookly/internal/api/hookly/v1"
	"hookly/internal/db"
	"hookly/internal/notify"
)

const (
	heartbeatInterval = 30 * time.Second
	staleTimeout      = 60 * time.Second
)

// Handler implements the RelayService.
type Handler struct {
	secret   string
	manager  *ConnectionManager
	queries  *db.Queries
	notifier notify.Notifier
}

// NewHandler creates a new relay handler.
func NewHandler(secret string, manager *ConnectionManager, queries *db.Queries, notifier notify.Notifier) *Handler {
	if notifier == nil {
		notifier = notify.NopNotifier{}
	}
	return &Handler{
		secret:   secret,
		manager:  manager,
		queries:  queries,
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

	// Validate authentication
	if !ValidateHMAC(connectReq.HubId, connectReq.Timestamp, connectReq.Signature, h.secret) {
		slog.Warn("relay auth failed", "hub_id", connectReq.HubId)
		if err := stream.Send(&hooklyv1.StreamResponse{
			Message: &hooklyv1.StreamResponse_ConnectResponse{
				ConnectResponse: &hooklyv1.ConnectResponse{
					Success: false,
					Error:   "authentication failed",
				},
			},
		}); err != nil {
			return err
		}
		return connect.NewError(connect.CodeUnauthenticated, errors.New("authentication failed"))
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

	// Mark as connected
	h.manager.SetConnected(connectReq.HubId)
	defer h.manager.SetDisconnected()

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
				h.handleAck(ctx, m.Ack)
			case *hooklyv1.StreamRequest_Heartbeat:
				h.manager.UpdateHeartbeat()
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
	sendCh := h.manager.SendCh()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case err := <-errCh:
			return err

		case webhook := <-sendCh:
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
			if h.manager.IsStale(staleTimeout) {
				slog.Warn("connection stale, closing")
				return connect.NewError(connect.CodeDeadlineExceeded, errors.New("connection stale"))
			}
		}
	}
}

func (h *Handler) handleAck(ctx context.Context, ack *hooklyv1.DeliveryAck) {
	slog.Info("received delivery ack",
		"webhook_id", ack.WebhookId,
		"success", ack.Success,
		"status_code", ack.StatusCode,
	)

	var err error
	if ack.Success {
		// Successfully delivered
		_, err = h.queries.MarkWebhookDelivered(ctx, ack.WebhookId)
	} else if ack.PermanentFailure {
		// Permanent failure (4xx) - stop retrying
		_, err = h.queries.MarkWebhookFailed(ctx, db.MarkWebhookFailedParams{
			ErrorMessage: stringToNullString(ack.ErrorMessage),
			ID:           ack.WebhookId,
		})
		if err == nil {
			// Send failure notification (fire and forget)
			go h.sendFailureNotification(ctx, ack.WebhookId, ack.ErrorMessage)
		}
	} else {
		// Transient failure (5xx or network error) - stay pending for retry
		_, err = h.queries.RecordWebhookAttempt(ctx, db.RecordWebhookAttemptParams{
			ErrorMessage: stringToNullString(ack.ErrorMessage),
			ID:           ack.WebhookId,
		})
		slog.Info("webhook will be retried after backoff",
			"webhook_id", ack.WebhookId,
			"error", ack.ErrorMessage,
		)
	}

	if err != nil {
		slog.Error("failed to update webhook status", "webhook_id", ack.WebhookId, "error", err)
	}
}

func (h *Handler) sendFailureNotification(ctx context.Context, webhookID, errorMsg string) {
	// Get webhook with endpoint info
	row, err := h.queries.GetWebhookWithEndpoint(ctx, webhookID)
	if err != nil {
		slog.Error("failed to get webhook for notification", "webhook_id", webhookID, "error", err)
		return
	}

	// Check if already notified
	if row.NotificationSent != 0 {
		return
	}

	// Parse received_at time
	receivedAt, _ := time.Parse("2006-01-02 15:04:05", row.ReceivedAt)

	info := notify.WebhookInfo{
		ID:             row.ID,
		EndpointID:     row.EndpointID,
		EndpointName:   row.EndpointName,
		DestinationURL: row.EndpointDestinationUrl,
		Attempts:       int(row.Attempts),
		Error:          errorMsg,
		ReceivedAt:     receivedAt,
	}

	if err := h.notifier.NotifyDeliveryFailure(ctx, info); err != nil {
		// Log but don't fail - notification is best-effort
		return
	}

	// Mark as notified
	if err := h.queries.MarkNotificationSent(ctx, webhookID); err != nil {
		slog.Error("failed to mark notification sent", "webhook_id", webhookID, "error", err)
	}
}

func stringToNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}
