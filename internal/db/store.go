package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// DefaultDestinationName names the destination created from a bare destination_url.
const DefaultDestinationName = "default"

var (
	// ErrLastDestination is returned when removing an endpoint's only destination.
	ErrLastDestination = errors.New("endpoint must keep at least one destination")
	// ErrLastEnabledDestination is returned when an endpoint would be left with no enabled destination.
	ErrLastEnabledDestination = errors.New("endpoint must keep at least one enabled destination (mute the endpoint instead)")
	// ErrDuplicateDestinationName is returned when a destination name is already used by the endpoint.
	ErrDuplicateDestinationName = errors.New("destination name already exists for this endpoint")
	// ErrInvalidDestination is returned when a destination has no name or URL.
	ErrInvalidDestination = errors.New("destination name and url are required")
	// ErrDeliveryNotPending is returned when an ack arrives for a delivery that is
	// no longer pending (duplicate or stale ack).
	ErrDeliveryNotPending = errors.New("delivery is not pending")
	// ErrDestinationMismatch is returned when a destination does not belong to the webhook's endpoint.
	ErrDestinationMismatch = errors.New("destination does not belong to the webhook's endpoint")
)

// DestinationSpec describes a destination to create.
type DestinationSpec struct {
	Name    string
	URL     string
	Enabled bool
}

// DeliveryOutcome is the result of one delivery attempt as reported by a home-hub.
type DeliveryOutcome struct {
	Success          bool
	PermanentFailure bool // 4xx - don't retry
	ErrorMessage     string
}

// Store wraps Queries with the multi-statement operations that keep
// destinations, deliveries and the derived webhook status consistent.
type Store struct {
	*Queries
	conn *sql.DB
}

// NewStore creates a Store on an open database.
func NewStore(conn *sql.DB) *Store {
	return &Store{Queries: New(conn), conn: conn}
}

// inTx runs fn in a transaction. The pool has a single connection, so fn must
// only use the Queries it is given.
func (s *Store) inTx(ctx context.Context, fn func(q *Queries) error) error {
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(s.Queries.WithTx(tx)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// CreateEndpointWithDestinations creates an endpoint and its destinations.
// params.DestinationUrl is ignored; the legacy column mirrors the first destination.
func (s *Store) CreateEndpointWithDestinations(ctx context.Context, params CreateEndpointParams, specs []DestinationSpec) (Endpoint, error) {
	if len(specs) == 0 {
		return Endpoint{}, ErrInvalidDestination
	}
	seen := make(map[string]bool, len(specs))
	anyEnabled := false
	for i := range specs {
		specs[i].Name = strings.TrimSpace(specs[i].Name)
		specs[i].URL = strings.TrimSpace(specs[i].URL)
		if specs[i].Name == "" || specs[i].URL == "" {
			return Endpoint{}, ErrInvalidDestination
		}
		if seen[specs[i].Name] {
			return Endpoint{}, ErrDuplicateDestinationName
		}
		seen[specs[i].Name] = true
		anyEnabled = anyEnabled || specs[i].Enabled
	}
	if !anyEnabled {
		return Endpoint{}, ErrLastEnabledDestination
	}

	var endpoint Endpoint
	err := s.inTx(ctx, func(q *Queries) error {
		params.DestinationUrl = specs[0].URL
		var err error
		endpoint, err = q.CreateEndpoint(ctx, params)
		if err != nil {
			return err
		}
		for i, spec := range specs {
			if _, err := insertDestination(ctx, q, endpoint.ID, spec, int64(i)); err != nil {
				return err
			}
		}
		return nil
	})
	return endpoint, err
}

// UpdateEndpointAndPrimary updates an endpoint. A destination_url change is
// applied to the endpoint's primary destination (legacy single-destination API).
func (s *Store) UpdateEndpointAndPrimary(ctx context.Context, params UpdateEndpointParams) (Endpoint, error) {
	var endpoint Endpoint
	err := s.inTx(ctx, func(q *Queries) error {
		var err error
		endpoint, err = q.UpdateEndpoint(ctx, params)
		if err != nil {
			return err
		}
		if !params.DestinationUrl.Valid {
			return nil
		}
		dests, err := q.ListDestinationsByEndpoint(ctx, endpoint.ID)
		if err != nil {
			return err
		}
		if len(dests) == 0 {
			_, err = insertDestination(ctx, q, endpoint.ID, DestinationSpec{
				Name: DefaultDestinationName, URL: params.DestinationUrl.String, Enabled: true,
			}, 0)
			return err
		}
		_, err = q.UpdateDestination(ctx, UpdateDestinationParams{ID: dests[0].ID, Url: params.DestinationUrl})
		return err
	})
	return endpoint, err
}

// AddDestination adds a destination to one of the user's endpoints. It only
// receives webhooks that arrive after it was added; no deliveries are created
// for older webhooks.
func (s *Store) AddDestination(ctx context.Context, userID, endpointID string, spec DestinationSpec) (Destination, error) {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.URL = strings.TrimSpace(spec.URL)
	if spec.Name == "" || spec.URL == "" {
		return Destination{}, ErrInvalidDestination
	}

	var dest Destination
	err := s.inTx(ctx, func(q *Queries) error {
		if _, err := q.GetEndpoint(ctx, GetEndpointParams{ID: endpointID, UserID: userID}); err != nil {
			return err
		}
		if err := checkDestinationName(ctx, q, endpointID, "", spec.Name); err != nil {
			return err
		}
		position, err := q.NextDestinationPosition(ctx, endpointID)
		if err != nil {
			return err
		}
		dest, err = insertDestination(ctx, q, endpointID, spec, position)
		if err != nil {
			return err
		}
		return syncLegacyDestinationURL(ctx, q, endpointID)
	})
	return dest, err
}

// UpdateDestination changes a destination's name, URL and/or enabled flag.
// Pending deliveries follow a URL change. Disabling pauses the destination:
// it is skipped for new webhooks and its pending deliveries are held.
func (s *Store) UpdateDestination(ctx context.Context, userID, id string, name, url *string, enabled *bool) (Destination, error) {
	var dest Destination
	err := s.inTx(ctx, func(q *Queries) error {
		current, err := q.GetDestinationForUser(ctx, GetDestinationForUserParams{ID: id, UserID: userID})
		if err != nil {
			return err
		}

		params := UpdateDestinationParams{ID: id}
		if name != nil {
			trimmed := strings.TrimSpace(*name)
			if trimmed == "" {
				return ErrInvalidDestination
			}
			if err := checkDestinationName(ctx, q, current.EndpointID, id, trimmed); err != nil {
				return err
			}
			params.Name = sql.NullString{String: trimmed, Valid: true}
		}
		if url != nil {
			trimmed := strings.TrimSpace(*url)
			if trimmed == "" {
				return ErrInvalidDestination
			}
			params.Url = sql.NullString{String: trimmed, Valid: true}
		}
		enabledChanged := enabled != nil && *enabled != (current.Enabled != 0)
		if enabled != nil {
			params.Enabled = sql.NullInt64{Int64: boolToInt(*enabled), Valid: true}
		}
		if enabledChanged && !*enabled {
			count, err := q.CountEnabledDestinationsByEndpoint(ctx, current.EndpointID)
			if err != nil {
				return err
			}
			if count <= 1 {
				return ErrLastEnabledDestination
			}
		}

		dest, err = q.UpdateDestination(ctx, params)
		if err != nil {
			return err
		}

		if enabledChanged {
			// Only enabled destinations count towards a webhook's derived status.
			if err := rollupDestinationWebhooks(ctx, q, id); err != nil {
				return err
			}
		}
		return syncLegacyDestinationURL(ctx, q, current.EndpointID)
	})
	return dest, err
}

// RemoveDestination deletes a destination. Its deliveries (including pending
// ones) are abandoned: they are deleted with it and the affected webhooks'
// status is re-derived from the destinations that remain.
func (s *Store) RemoveDestination(ctx context.Context, userID, id string) (endpointID string, err error) {
	err = s.inTx(ctx, func(q *Queries) error {
		dest, err := q.GetDestinationForUser(ctx, GetDestinationForUserParams{ID: id, UserID: userID})
		if err != nil {
			return err
		}
		endpointID = dest.EndpointID

		dests, err := q.ListDestinationsByEndpoint(ctx, endpointID)
		if err != nil {
			return err
		}
		if len(dests) <= 1 {
			return ErrLastDestination
		}
		enabledRemaining := 0
		for _, d := range dests {
			if d.ID != id && d.Enabled != 0 {
				enabledRemaining++
			}
		}
		if enabledRemaining == 0 {
			return ErrLastEnabledDestination
		}

		webhookIDs, err := q.ListWebhookIDsByDestination(ctx, id)
		if err != nil {
			return err
		}
		if err := q.DeleteDestination(ctx, id); err != nil {
			return err
		}
		for _, webhookID := range webhookIDs {
			if err := rollupWebhook(ctx, q, webhookID); err != nil {
				return err
			}
		}
		return syncLegacyDestinationURL(ctx, q, endpointID)
	})
	return endpointID, err
}

// CreateWebhookWithDeliveries stores a webhook and fans it out: one pending
// delivery per destination that is enabled right now.
func (s *Store) CreateWebhookWithDeliveries(ctx context.Context, params CreateWebhookParams) (Webhook, error) {
	var webhook Webhook
	err := s.inTx(ctx, func(q *Queries) error {
		var err error
		webhook, err = q.CreateWebhook(ctx, params)
		if err != nil {
			return err
		}
		dests, err := q.ListEnabledDestinationsByEndpoint(ctx, params.EndpointID)
		if err != nil {
			return err
		}
		if len(dests) == 0 {
			slog.Warn("endpoint has no enabled destinations, webhook stored without deliveries",
				"webhook_id", webhook.ID,
				"endpoint_id", params.EndpointID,
			)
		}
		for _, dest := range dests {
			if _, err := insertDelivery(ctx, q, webhook.ID, dest.ID); err != nil {
				return err
			}
		}
		return nil
	})
	return webhook, err
}

// ResolveAckDelivery finds the delivery an ack refers to. Hubs that predate
// fan-out don't echo a delivery ID; they are only ever sent the primary
// destination, so the ack falls back to the webhook's first pending delivery
// (which is its only one for single-destination endpoints).
func (s *Store) ResolveAckDelivery(ctx context.Context, webhookID, deliveryID string) (string, error) {
	if deliveryID != "" {
		return deliveryID, nil
	}
	pending, err := s.ListPendingDeliveriesByWebhook(ctx, webhookID)
	if err != nil {
		return "", err
	}
	if len(pending) == 0 {
		return "", ErrDeliveryNotPending
	}
	return pending[0].ID, nil
}

// ApplyDeliveryOutcome advances a pending delivery's state and re-derives the
// webhook's status:
//
//	success           -> delivered
//	permanent failure -> failed (no retry)
//	otherwise         -> stays pending, attempts+1 (retried after backoff)
//
// Returns ErrDeliveryNotPending for duplicate or stale acks, which change nothing.
func (s *Store) ApplyDeliveryOutcome(ctx context.Context, deliveryID string, outcome DeliveryOutcome) (Delivery, error) {
	var delivery Delivery
	err := s.inTx(ctx, func(q *Queries) error {
		var err error
		errorMessage := sql.NullString{String: outcome.ErrorMessage, Valid: outcome.ErrorMessage != ""}
		switch {
		case outcome.Success:
			delivery, err = q.MarkDeliveryDelivered(ctx, deliveryID)
		case outcome.PermanentFailure:
			delivery, err = q.MarkDeliveryFailed(ctx, MarkDeliveryFailedParams{ErrorMessage: errorMessage, ID: deliveryID})
		default:
			delivery, err = q.RecordDeliveryAttempt(ctx, RecordDeliveryAttemptParams{ErrorMessage: errorMessage, ID: deliveryID})
		}
		if errors.Is(err, sql.ErrNoRows) {
			return ErrDeliveryNotPending
		}
		if err != nil {
			return err
		}
		return rollupWebhook(ctx, q, delivery.WebhookID)
	})
	return delivery, err
}

// ReplayWebhook resets deliveries to pending. With destinationID empty, every
// destination the webhook was fanned out to is replayed. With a destinationID,
// only that destination is replayed; if it never received the webhook (it was
// added later) a delivery is created for it.
func (s *Store) ReplayWebhook(ctx context.Context, userID, webhookID, destinationID string) (Webhook, error) {
	var webhook Webhook
	err := s.inTx(ctx, func(q *Queries) error {
		// Validates that the webhook belongs to one of the user's endpoints
		wh, err := q.GetWebhook(ctx, GetWebhookParams{ID: webhookID, UserID: userID})
		if err != nil {
			return err
		}

		if destinationID == "" {
			deliveries, err := q.ListDeliveriesByWebhook(ctx, webhookID)
			if err != nil {
				return err
			}
			for _, d := range deliveries {
				if _, err := q.ResetDeliveryForReplay(ctx, d.ID); err != nil {
					return err
				}
			}
			if len(deliveries) == 0 {
				// Every destination it was delivered to has since been removed.
				dests, err := q.ListEnabledDestinationsByEndpoint(ctx, wh.EndpointID)
				if err != nil {
					return err
				}
				for _, dest := range dests {
					if _, err := insertDelivery(ctx, q, webhookID, dest.ID); err != nil {
						return err
					}
				}
			}
		} else {
			dest, err := q.GetDestination(ctx, destinationID)
			if err != nil {
				return err
			}
			if dest.EndpointID != wh.EndpointID {
				return ErrDestinationMismatch
			}
			existing, err := q.GetDeliveryByWebhookAndDestination(ctx, GetDeliveryByWebhookAndDestinationParams{
				WebhookID:     webhookID,
				DestinationID: destinationID,
			})
			switch {
			case errors.Is(err, sql.ErrNoRows):
				_, err = insertDelivery(ctx, q, webhookID, destinationID)
			case err == nil:
				_, err = q.ResetDeliveryForReplay(ctx, existing.ID)
			}
			if err != nil {
				return err
			}
		}

		if err := rollupWebhook(ctx, q, webhookID); err != nil {
			return err
		}
		webhook, err = q.GetWebhookByID(ctx, webhookID)
		return err
	})
	return webhook, err
}

// MarkDeadLetters dead-letters pending deliveries whose webhook is older than
// 7 days. Each destination expires on its own; returns the number of deliveries.
func (s *Store) MarkDeadLetters(ctx context.Context) (int64, error) {
	var count int64
	err := s.inTx(ctx, func(q *Queries) error {
		expired, err := q.ListExpiredPendingDeliveries(ctx)
		if err != nil {
			return err
		}
		webhookIDs := make(map[string]bool)
		for _, d := range expired {
			if err := q.MarkDeliveryDeadLetter(ctx, d.ID); err != nil {
				return err
			}
			webhookIDs[d.WebhookID] = true
		}
		for webhookID := range webhookIDs {
			if err := rollupWebhook(ctx, q, webhookID); err != nil {
				return err
			}
		}
		count = int64(len(expired))
		return nil
	})
	return count, err
}

func insertDestination(ctx context.Context, q *Queries, endpointID string, spec DestinationSpec, position int64) (Destination, error) {
	id, err := gonanoid.New()
	if err != nil {
		return Destination{}, err
	}
	return q.CreateDestination(ctx, CreateDestinationParams{
		ID:         id,
		EndpointID: endpointID,
		Name:       spec.Name,
		Url:        spec.URL,
		Enabled:    boolToInt(spec.Enabled),
		Position:   position,
	})
}

func insertDelivery(ctx context.Context, q *Queries, webhookID, destinationID string) (Delivery, error) {
	id, err := gonanoid.New()
	if err != nil {
		return Delivery{}, err
	}
	return q.CreateDelivery(ctx, CreateDeliveryParams{
		ID:            id,
		WebhookID:     webhookID,
		DestinationID: destinationID,
	})
}

// checkDestinationName rejects a name already used by another destination of the endpoint.
func checkDestinationName(ctx context.Context, q *Queries, endpointID, excludeID, name string) error {
	dests, err := q.ListDestinationsByEndpoint(ctx, endpointID)
	if err != nil {
		return err
	}
	for _, d := range dests {
		if d.ID != excludeID && d.Name == name {
			return ErrDuplicateDestinationName
		}
	}
	return nil
}

// syncLegacyDestinationURL mirrors the primary destination into endpoints.destination_url.
func syncLegacyDestinationURL(ctx context.Context, q *Queries, endpointID string) error {
	dests, err := q.ListDestinationsByEndpoint(ctx, endpointID)
	if err != nil {
		return err
	}
	if len(dests) == 0 {
		return nil
	}
	return q.SetEndpointLegacyDestinationURL(ctx, SetEndpointLegacyDestinationURLParams{
		DestinationUrl: dests[0].Url,
		ID:             endpointID,
	})
}

func rollupDestinationWebhooks(ctx context.Context, q *Queries, destinationID string) error {
	webhookIDs, err := q.ListWebhookIDsByDestination(ctx, destinationID)
	if err != nil {
		return err
	}
	for _, webhookID := range webhookIDs {
		if err := rollupWebhook(ctx, q, webhookID); err != nil {
			return err
		}
	}
	return nil
}

// rollupWebhook stores the webhook status derived from its deliveries.
func rollupWebhook(ctx context.Context, q *Queries, webhookID string) error {
	deliveries, err := q.ListDeliveriesByWebhook(ctx, webhookID)
	if err != nil {
		return err
	}

	states := make([]DeliveryState, len(deliveries))
	for i, d := range deliveries {
		states[i] = DeliveryState{
			Status:        d.Status,
			Attempts:      d.Attempts,
			LastAttemptAt: d.LastAttemptAt,
			DeliveredAt:   d.DeliveredAt,
			ErrorMessage:  d.ErrorMessage,
			Enabled:       d.DestinationEnabled != 0,
		}
	}

	rollup, ok := Rollup(states)
	if !ok {
		// Every destination this webhook was fanned out to has been removed.
		// Whatever was delivered stays delivered; anything unfinished is abandoned.
		wh, err := q.GetWebhookByID(ctx, webhookID)
		if err != nil {
			return err
		}
		if wh.Status != StatusPending {
			return nil
		}
		lastAttemptAt := wh.LastAttemptAt
		if !lastAttemptAt.Valid {
			// Retention cleanup of failed webhooks keys off last_attempt_at.
			lastAttemptAt = sql.NullString{String: wh.ReceivedAt, Valid: true}
		}
		rollup = WebhookRollup{
			Status:        StatusFailed,
			Attempts:      wh.Attempts,
			LastAttemptAt: lastAttemptAt,
			ErrorMessage:  sql.NullString{String: "abandoned: destination removed before delivery", Valid: true},
		}
	}

	if err := q.UpdateWebhookRollup(ctx, UpdateWebhookRollupParams{
		Status:        rollup.Status,
		Attempts:      rollup.Attempts,
		LastAttemptAt: rollup.LastAttemptAt,
		DeliveredAt:   rollup.DeliveredAt,
		ErrorMessage:  rollup.ErrorMessage,
		ID:            webhookID,
	}); err != nil {
		return fmt.Errorf("update webhook rollup: %w", err)
	}
	return nil
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
