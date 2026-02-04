package relay

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	hooklyv1 "hooks.dx314.com/internal/api/hookly/v1"
	"hooks.dx314.com/internal/db"

	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	dispatchInterval = 1 * time.Second
	batchSize        = 100
)

// Dispatcher watches for pending webhooks and sends them to the appropriate home-hub.
type Dispatcher struct {
	queries *db.Queries
	manager *ConnectionManager
}

// NewDispatcher creates a new webhook dispatcher.
func NewDispatcher(queries *db.Queries, manager *ConnectionManager) *Dispatcher {
	return &Dispatcher{
		queries: queries,
		manager: manager,
	}
}

// Run starts the dispatcher loop. Blocks until context is cancelled.
func (d *Dispatcher) Run(ctx context.Context) error {
	ticker := time.NewTicker(dispatchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if d.manager.IsAnyConnected() {
				if err := d.dispatch(ctx); err != nil {
					slog.Error("dispatch error", "error", err)
				}
			}
		}
	}
}

func (d *Dispatcher) dispatch(ctx context.Context) error {
	// Get pending webhooks
	webhooks, err := d.queries.GetPendingWebhooks(ctx, batchSize)
	if err != nil {
		return err
	}

	for _, wh := range webhooks {
		// Look up which hub handles this endpoint
		conn := d.manager.GetHubForEndpoint(wh.EndpointID)
		if conn == nil {
			// No hub registered for this endpoint, skip
			continue
		}

		// Parse headers JSON
		var headers map[string]string
		if err := json.Unmarshal([]byte(wh.Headers), &headers); err != nil {
			slog.Warn("failed to parse headers", "webhook_id", wh.ID, "error", err)
			headers = make(map[string]string)
		}

		// Parse received_at timestamp
		receivedAt, err := time.Parse("2006-01-02 15:04:05", wh.ReceivedAt)
		if err != nil {
			receivedAt = time.Now()
		}

		envelope := &hooklyv1.WebhookEnvelope{
			Id:             wh.ID,
			EndpointId:     wh.EndpointID,
			DestinationUrl: wh.DestinationUrl,
			ReceivedAt:     timestamppb.New(receivedAt),
			Headers:        headers,
			Payload:        wh.Payload,
			Attempt:        int32(wh.Attempts) + 1,
		}

		if !conn.Send(envelope) {
			slog.Warn("failed to queue webhook for delivery",
				"webhook_id", wh.ID,
				"hub_id", conn.HubID(),
			)
			continue
		}

		slog.Debug("queued webhook for delivery",
			"webhook_id", wh.ID,
			"endpoint_id", wh.EndpointID,
			"hub_id", conn.HubID(),
			"attempt", envelope.Attempt,
		)
	}

	return nil
}
