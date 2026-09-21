package relay

import (
	"context"
	"path/filepath"
	"testing"

	hooklyv1 "hooks.dx314.com/internal/api/hookly/v1"
	"hooks.dx314.com/internal/db"
)

func newFanoutStore(t *testing.T) *db.Store {
	t.Helper()
	ctx := context.Background()
	conn, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	store := db.NewStore(conn)

	if _, err := store.CreateEndpointWithDestinations(ctx, db.CreateEndpointParams{
		ID: "ep", UserID: "user-1", Name: "ep", ProviderType: "telegram", SignatureSecretEncrypted: []byte("x"),
	}, []db.DestinationSpec{
		{Name: "otto", URL: "http://localhost/otto", Enabled: true},
		{Name: "schoolboy", URL: "http://localhost/schoolboy", Enabled: true},
	}); err != nil {
		t.Fatalf("create endpoint: %v", err)
	}
	if _, err := store.CreateWebhookWithDeliveries(ctx, db.CreateWebhookParams{
		ID: "w1", EndpointID: "ep", Headers: `{"X-Telegram-Bot-Api-Secret-Token":"s"}`, Payload: []byte("{}"), SignatureValid: 1,
	}); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	return store
}

func drain(conn *HubConnection) []*hooklyv1.WebhookEnvelope {
	var out []*hooklyv1.WebhookEnvelope
	for {
		select {
		case env := <-conn.SendCh():
			out = append(out, env)
		default:
			return out
		}
	}
}

func TestDispatchFansOutToCapableHub(t *testing.T) {
	ctx := context.Background()
	store := newFanoutStore(t)
	mgr := NewConnectionManager()
	conn := mgr.AddConnection("hub", []string{"ep"}, []string{CapabilityFanout})
	d := NewDispatcher(store, mgr)

	if err := d.dispatch(ctx); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	envelopes := drain(conn)
	if len(envelopes) != 2 {
		t.Fatalf("got %d envelopes, want one per destination", len(envelopes))
	}
	for i, want := range []struct {
		name    string
		primary bool
	}{{"otto", true}, {"schoolboy", false}} {
		env := envelopes[i]
		if env.Id != "w1" || env.DestinationName != want.name || env.DestinationPrimary != want.primary ||
			env.DeliveryId == "" || env.DestinationId == "" || env.DestinationUrl != "http://localhost/"+want.name {
			t.Errorf("envelope %d = %+v", i, env)
		}
		if env.Headers["X-Telegram-Bot-Api-Secret-Token"] != "s" {
			t.Errorf("envelope %d lost the original headers: %v", i, env.Headers)
		}
	}

	// Unacked deliveries are in flight: the next tick must not send them again
	if err := d.dispatch(ctx); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if again := drain(conn); len(again) != 0 {
		t.Errorf("in-flight deliveries were sent again: %d", len(again))
	}

	// Acking one destination doesn't affect the other
	h := NewHandler(nil, mgr, store, nil)
	h.handleAck(ctx, conn, &hooklyv1.DeliveryAck{WebhookId: "w1", DeliveryId: envelopes[0].DeliveryId, Success: true})
	deliveries, _ := store.ListDeliveriesByWebhook(ctx, "w1")
	if deliveries[0].Status != db.StatusDelivered || deliveries[1].Status != db.StatusPending {
		t.Errorf("statuses = %s, %s", deliveries[0].Status, deliveries[1].Status)
	}
	if conn.IsInFlight(envelopes[0].DeliveryId) || !conn.IsInFlight(envelopes[1].DeliveryId) {
		t.Error("in-flight tracking not updated by ack")
	}
}

// A hub that predates fan-out only ever gets the primary destination, and its
// ack (webhook ID only) resolves to that delivery.
func TestDispatchToLegacyHub(t *testing.T) {
	ctx := context.Background()
	store := newFanoutStore(t)
	mgr := NewConnectionManager()
	conn := mgr.AddConnection("old-hub", []string{"ep"}, nil)
	d := NewDispatcher(store, mgr)

	if err := d.dispatch(ctx); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	envelopes := drain(conn)
	if len(envelopes) != 1 || envelopes[0].DestinationName != "otto" {
		t.Fatalf("legacy hub got %d envelopes (%v), want only the primary", len(envelopes), envelopes)
	}

	h := NewHandler(nil, mgr, store, nil)
	h.handleAck(ctx, conn, &hooklyv1.DeliveryAck{WebhookId: "w1", Success: true})

	deliveries, _ := store.ListDeliveriesByWebhook(ctx, "w1")
	if deliveries[0].DestinationName != "otto" || deliveries[0].Status != db.StatusDelivered {
		t.Errorf("primary delivery = %s %s, want delivered", deliveries[0].DestinationName, deliveries[0].Status)
	}
	if deliveries[1].Status != db.StatusPending || deliveries[1].Attempts != 0 {
		t.Errorf("secondary delivery = %s attempts %d, want untouched", deliveries[1].Status, deliveries[1].Attempts)
	}

	// The secondary destination waits for a hub that supports fan-out
	if err := d.dispatch(ctx); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if held := drain(conn); len(held) != 0 {
		t.Errorf("legacy hub was sent a secondary destination")
	}
	upgraded := mgr.AddConnection("old-hub", []string{"ep"}, []string{CapabilityFanout})
	if err := d.dispatch(ctx); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if sent := drain(upgraded); len(sent) != 1 || sent[0].DestinationName != "schoolboy" {
		t.Errorf("upgraded hub got %v, want schoolboy", sent)
	}
}
