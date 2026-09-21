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

// Dispatcher watches for pending deliveries and sends them to the appropriate home-hub.
// One envelope is sent per (webhook, destination).
type Dispatcher struct {
	store   *db.Store
	manager *ConnectionManager
}

// NewDispatcher creates a new webhook dispatcher.
func NewDispatcher(store *db.Store, manager *ConnectionManager) *Dispatcher {
	return &Dispatcher{
		store:   store,
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
	// Get the oldest ready delivery of every destination
	deliveries, err := d.store.GetDispatchableDeliveries(ctx, batchSize)
	if err != nil {
		return err
	}

	primaries := make(map[string]string) // endpointID → primary destination ID

	for _, dl := range deliveries {
		// Look up which hub handles this endpoint
		conn := d.manager.GetHubForEndpoint(dl.EndpointID)
		if conn == nil {
			// No hub registered for this endpoint, skip
			continue
		}

		// Sent and not acked yet - don't deliver twice
		if conn.IsInFlight(dl.DeliveryID) {
			continue
		}

		primaryID, ok := primaries[dl.EndpointID]
		if !ok {
			primaryID, err = d.primaryDestinationID(ctx, dl.EndpointID)
			if err != nil {
				slog.Warn("failed to look up primary destination", "endpoint_id", dl.EndpointID, "error", err)
				continue
			}
			primaries[dl.EndpointID] = primaryID
		}
		isPrimary := dl.DestinationID == primaryID

		// A hub that predates fan-out would ack by webhook ID only and may override
		// the URL per endpoint, so it is only trusted with the primary destination.
		// Other destinations stay pending until the hub is upgraded.
		if !isPrimary && !conn.SupportsFanout() {
			continue
		}

		// Parse headers JSON
		var headers map[string]string
		if err := json.Unmarshal([]byte(dl.Headers), &headers); err != nil {
			slog.Warn("failed to parse headers", "webhook_id", dl.WebhookID, "error", err)
			headers = make(map[string]string)
		}

		// Parse received_at timestamp
		receivedAt, err := time.Parse("2006-01-02 15:04:05", dl.ReceivedAt)
		if err != nil {
			receivedAt = time.Now()
		}

		envelope := &hooklyv1.WebhookEnvelope{
			Id:                 dl.WebhookID,
			EndpointId:         dl.EndpointID,
			DestinationUrl:     dl.DestinationUrl,
			ReceivedAt:         timestamppb.New(receivedAt),
			Headers:            headers,
			Payload:            dl.Payload,
			Attempt:            int32(dl.Attempts) + 1,
			DeliveryId:         dl.DeliveryID,
			DestinationId:      dl.DestinationID,
			DestinationName:    dl.DestinationName,
			DestinationPrimary: isPrimary,
		}

		conn.MarkInFlight(dl.DeliveryID)
		if !conn.Send(envelope) {
			conn.ClearInFlight(dl.DeliveryID)
			slog.Warn("failed to queue webhook for delivery",
				"webhook_id", dl.WebhookID,
				"delivery_id", dl.DeliveryID,
				"hub_id", conn.HubID(),
			)
			continue
		}

		slog.Debug("queued webhook for delivery",
			"webhook_id", dl.WebhookID,
			"delivery_id", dl.DeliveryID,
			"endpoint_id", dl.EndpointID,
			"destination", dl.DestinationName,
			"hub_id", conn.HubID(),
			"attempt", envelope.Attempt,
		)
	}

	return nil
}

// primaryDestinationID returns the endpoint's first destination.
func (d *Dispatcher) primaryDestinationID(ctx context.Context, endpointID string) (string, error) {
	dests, err := d.store.ListDestinationsByEndpoint(ctx, endpointID)
	if err != nil {
		return "", err
	}
	if len(dests) == 0 {
		return "", nil
	}
	return dests[0].ID, nil
}
