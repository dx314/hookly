package db_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"hooks.dx314.com/internal/db"
)

const testUser = "user-1"

func newTestStore(t *testing.T) (*db.Store, *sql.DB) {
	t.Helper()
	conn, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return db.NewStore(conn), conn
}

// createTestEndpoint creates an endpoint with the named destinations and returns them by name.
func createTestEndpoint(t *testing.T, store *db.Store, id string, names ...string) map[string]db.Destination {
	t.Helper()
	ctx := context.Background()
	specs := make([]db.DestinationSpec, len(names))
	for i, name := range names {
		specs[i] = db.DestinationSpec{Name: name, URL: "http://localhost/" + name, Enabled: true}
	}
	if _, err := store.CreateEndpointWithDestinations(ctx, db.CreateEndpointParams{
		ID: id, UserID: testUser, Name: id, ProviderType: "generic", SignatureSecretEncrypted: []byte("x"),
	}, specs); err != nil {
		t.Fatalf("create endpoint: %v", err)
	}
	dests, err := store.ListDestinationsByEndpoint(ctx, id)
	if err != nil {
		t.Fatalf("list destinations: %v", err)
	}
	byName := make(map[string]db.Destination)
	for _, d := range dests {
		byName[d.Name] = d
	}
	return byName
}

func receiveWebhook(t *testing.T, store *db.Store, endpointID, id string) {
	t.Helper()
	if _, err := store.CreateWebhookWithDeliveries(context.Background(), db.CreateWebhookParams{
		ID: id, EndpointID: endpointID, Headers: "{}", Payload: []byte(id), SignatureValid: 1,
	}); err != nil {
		t.Fatalf("create webhook %s: %v", id, err)
	}
}

// deliveryFor returns the delivery of a webhook for a destination.
func deliveryFor(t *testing.T, store *db.Store, webhookID, destinationID string) db.Delivery {
	t.Helper()
	d, err := store.GetDeliveryByWebhookAndDestination(context.Background(), db.GetDeliveryByWebhookAndDestinationParams{
		WebhookID: webhookID, DestinationID: destinationID,
	})
	if err != nil {
		t.Fatalf("get delivery %s/%s: %v", webhookID, destinationID, err)
	}
	return d
}

func webhookStatus(t *testing.T, store *db.Store, id string) string {
	t.Helper()
	wh, err := store.GetWebhookByID(context.Background(), id)
	if err != nil {
		t.Fatalf("get webhook: %v", err)
	}
	return wh.Status
}

// dispatchable returns "webhook→destination" for everything ready to send, in order.
func dispatchable(t *testing.T, store *db.Store) []string {
	t.Helper()
	rows, err := store.GetDispatchableDeliveries(context.Background(), 100)
	if err != nil {
		t.Fatalf("get dispatchable: %v", err)
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.WebhookID + "→" + r.DestinationName
	}
	return out
}

// skipBackoff makes every pending delivery immediately retryable.
func skipBackoff(t *testing.T, conn *sql.DB) {
	t.Helper()
	if _, err := conn.Exec("UPDATE deliveries SET last_attempt_at = datetime('now', '-2 hours') WHERE last_attempt_at IS NOT NULL AND status = 'pending'"); err != nil {
		t.Fatalf("skip backoff: %v", err)
	}
}

func apply(t *testing.T, store *db.Store, deliveryID string, outcome db.DeliveryOutcome) db.Delivery {
	t.Helper()
	d, err := store.ApplyDeliveryOutcome(context.Background(), deliveryID, outcome)
	if err != nil {
		t.Fatalf("apply outcome: %v", err)
	}
	return d
}

var (
	success   = db.DeliveryOutcome{Success: true}
	transient = db.DeliveryOutcome{ErrorMessage: "HTTP 500"}
	permanent = db.DeliveryOutcome{PermanentFailure: true, ErrorMessage: "HTTP 404"}
)

func assertEqual(t *testing.T, what string, got, want any) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

func TestDeliveryStateMachine(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	dests := createTestEndpoint(t, store, "ep", "otto", "schoolboy")
	receiveWebhook(t, store, "ep", "w1")

	otto := deliveryFor(t, store, "w1", dests["otto"].ID)
	schoolboy := deliveryFor(t, store, "w1", dests["schoolboy"].ID)
	assertEqual(t, "initial status", otto.Status, db.StatusPending)
	assertEqual(t, "webhook status", webhookStatus(t, store, "w1"), db.StatusPending)

	// Transient failure: stays pending, attempt recorded, other destination untouched
	got := apply(t, store, schoolboy.ID, transient)
	assertEqual(t, "status after 5xx", got.Status, db.StatusPending)
	assertEqual(t, "attempts after 5xx", got.Attempts, 1)
	assertEqual(t, "error after 5xx", got.ErrorMessage.String, "HTTP 500")
	assertEqual(t, "otto attempts", deliveryFor(t, store, "w1", dests["otto"].ID).Attempts, 0)

	// One delivered, one still retrying: webhook stays pending
	got = apply(t, store, otto.ID, success)
	assertEqual(t, "status after 2xx", got.Status, db.StatusDelivered)
	assertEqual(t, "webhook status", webhookStatus(t, store, "w1"), db.StatusPending)

	// Duplicate / stale ack changes nothing
	if _, err := store.ApplyDeliveryOutcome(ctx, otto.ID, transient); !errors.Is(err, db.ErrDeliveryNotPending) {
		t.Errorf("duplicate ack error = %v, want ErrDeliveryNotPending", err)
	}
	after := deliveryFor(t, store, "w1", dests["otto"].ID)
	assertEqual(t, "status after duplicate ack", after.Status, db.StatusDelivered)
	assertEqual(t, "attempts after duplicate ack", after.Attempts, 1)

	// Recovery: all delivered -> webhook delivered, error cleared
	got = apply(t, store, schoolboy.ID, success)
	assertEqual(t, "attempts after recovery", got.Attempts, 2)
	assertEqual(t, "error after recovery", got.ErrorMessage.Valid, false)
	wh, _ := store.GetWebhookByID(ctx, "w1")
	assertEqual(t, "webhook status", wh.Status, db.StatusDelivered)
	assertEqual(t, "webhook attempts", wh.Attempts, 2)
	assertEqual(t, "webhook delivered_at set", wh.DeliveredAt.Valid, true)
	assertEqual(t, "webhook error", wh.ErrorMessage.Valid, false)

	// Permanent failure: failed, no retry, webhook failed even though otto delivered
	receiveWebhook(t, store, "ep", "w2")
	apply(t, store, deliveryFor(t, store, "w2", dests["otto"].ID).ID, success)
	got = apply(t, store, deliveryFor(t, store, "w2", dests["schoolboy"].ID).ID, permanent)
	assertEqual(t, "status after 4xx", got.Status, db.StatusFailed)
	assertEqual(t, "webhook status", webhookStatus(t, store, "w2"), db.StatusFailed)
	assertEqual(t, "nothing left to dispatch", len(dispatchable(t, store)), 0)
}

func TestDestinationsAreIndependentAndInOrder(t *testing.T) {
	store, conn := newTestStore(t)
	dests := createTestEndpoint(t, store, "ep", "otto", "schoolboy")
	for _, id := range []string{"w1", "w2", "w3"} {
		receiveWebhook(t, store, "ep", id)
	}

	// Only the oldest webhook of each destination is ready
	assertEqual(t, "initial", dispatchable(t, store), []string{"w1→otto", "w1→schoolboy"})

	// schoolboy is down: it backs off, otto carries on through the queue
	apply(t, store, deliveryFor(t, store, "w1", dests["schoolboy"].ID).ID, transient)
	apply(t, store, deliveryFor(t, store, "w1", dests["otto"].ID).ID, success)
	assertEqual(t, "otto moves on, schoolboy backs off", dispatchable(t, store), []string{"w2→otto"})

	apply(t, store, deliveryFor(t, store, "w2", dests["otto"].ID).ID, success)
	assertEqual(t, "otto continues", dispatchable(t, store), []string{"w3→otto"})
	apply(t, store, deliveryFor(t, store, "w3", dests["otto"].ID).ID, success)
	assertEqual(t, "otto done, schoolboy still backing off", dispatchable(t, store), []string{})

	// After the backoff schoolboy catches up strictly in order, never skipping w1
	for _, id := range []string{"w1", "w2", "w3"} {
		skipBackoff(t, conn)
		assertEqual(t, "schoolboy next", dispatchable(t, store), []string{id + "→schoolboy"})
		apply(t, store, deliveryFor(t, store, id, dests["schoolboy"].ID).ID, success)
	}
	assertEqual(t, "all done", dispatchable(t, store), []string{})
	for _, id := range []string{"w1", "w2", "w3"} {
		assertEqual(t, id+" status", webhookStatus(t, store, id), db.StatusDelivered)
		assertEqual(t, id+" otto attempts", deliveryFor(t, store, id, dests["otto"].ID).Attempts, 1)
	}
}

func TestBackoffCapsAtOneHour(t *testing.T) {
	store, conn := newTestStore(t)
	dests := createTestEndpoint(t, store, "ep", "otto")
	receiveWebhook(t, store, "ep", "w1")
	d := deliveryFor(t, store, "w1", dests["otto"].ID)

	// 100 attempts would overflow 1 << attempts; the delay must stay capped at 1h
	if _, err := conn.Exec("UPDATE deliveries SET attempts = 100, last_attempt_at = datetime('now', '-59 minutes') WHERE id = ?", d.ID); err != nil {
		t.Fatal(err)
	}
	assertEqual(t, "59 minutes after attempt 100", dispatchable(t, store), []string{})
	if _, err := conn.Exec("UPDATE deliveries SET last_attempt_at = datetime('now', '-61 minutes') WHERE id = ?", d.ID); err != nil {
		t.Fatal(err)
	}
	assertEqual(t, "61 minutes after attempt 100", dispatchable(t, store), []string{"w1→otto"})
}

func TestDestinationAddedLaterGetsNoOldTraffic(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	createTestEndpoint(t, store, "ep", "otto")
	receiveWebhook(t, store, "ep", "w1")

	added, err := store.AddDestination(ctx, testUser, "ep", db.DestinationSpec{Name: "schoolboy", URL: "http://localhost/schoolboy", Enabled: true})
	if err != nil {
		t.Fatalf("add destination: %v", err)
	}
	receiveWebhook(t, store, "ep", "w2")

	assertEqual(t, "dispatchable", dispatchable(t, store), []string{"w1→otto", "w2→schoolboy"})
	if _, err := store.GetDeliveryByWebhookAndDestination(ctx, db.GetDeliveryByWebhookAndDestinationParams{WebhookID: "w1", DestinationID: added.ID}); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("old webhook has a delivery for the new destination (err = %v)", err)
	}

	// Validation
	if _, err := store.AddDestination(ctx, testUser, "ep", db.DestinationSpec{Name: "schoolboy", URL: "http://x", Enabled: true}); !errors.Is(err, db.ErrDuplicateDestinationName) {
		t.Errorf("duplicate name error = %v", err)
	}
	if _, err := store.AddDestination(ctx, testUser, "ep", db.DestinationSpec{Name: "", URL: "http://x"}); !errors.Is(err, db.ErrInvalidDestination) {
		t.Errorf("empty name error = %v", err)
	}
	if _, err := store.AddDestination(ctx, testUser, "nope", db.DestinationSpec{Name: "a", URL: "http://x"}); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("unknown endpoint error = %v", err)
	}

	// A disabled destination is skipped for new webhooks and its pending deliveries are held
	off := false
	if _, err := store.UpdateDestination(ctx, testUser, added.ID, nil, nil, &off); err != nil {
		t.Fatalf("disable: %v", err)
	}
	receiveWebhook(t, store, "ep", "w3")
	assertEqual(t, "dispatchable while disabled", dispatchable(t, store), []string{"w1→otto"})
	if _, err := store.GetDeliveryByWebhookAndDestination(ctx, db.GetDeliveryByWebhookAndDestinationParams{WebhookID: "w3", DestinationID: added.ID}); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("disabled destination got a delivery (err = %v)", err)
	}
}

func TestRemoveDestinationAbandonsItsDeliveries(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	dests := createTestEndpoint(t, store, "ep", "otto", "schoolboy")
	receiveWebhook(t, store, "ep", "w1")
	receiveWebhook(t, store, "ep", "w2")
	apply(t, store, deliveryFor(t, store, "w1", dests["otto"].ID).ID, success)
	apply(t, store, deliveryFor(t, store, "w1", dests["schoolboy"].ID).ID, transient)
	assertEqual(t, "w1 before", webhookStatus(t, store, "w1"), db.StatusPending)

	endpointID, err := store.RemoveDestination(ctx, testUser, dests["schoolboy"].ID)
	if err != nil {
		t.Fatalf("remove destination: %v", err)
	}
	assertEqual(t, "endpoint", endpointID, "ep")

	// schoolboy's pending deliveries are gone; w1 is now fully delivered, w2 still owed to otto
	assertEqual(t, "w1 after", webhookStatus(t, store, "w1"), db.StatusDelivered)
	assertEqual(t, "w2 after", webhookStatus(t, store, "w2"), db.StatusPending)
	assertEqual(t, "dispatchable", dispatchable(t, store), []string{"w2→otto"})

	// The last destination can't be removed or disabled
	if _, err := store.RemoveDestination(ctx, testUser, dests["otto"].ID); !errors.Is(err, db.ErrLastDestination) {
		t.Errorf("remove last destination error = %v", err)
	}
	off := false
	if _, err := store.UpdateDestination(ctx, testUser, dests["otto"].ID, nil, nil, &off); !errors.Is(err, db.ErrLastEnabledDestination) {
		t.Errorf("disable last destination error = %v", err)
	}
}

func TestRemovePrimaryPromotesNextDestination(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	dests := createTestEndpoint(t, store, "ep", "otto", "schoolboy")

	ep, _ := store.GetEndpoint(ctx, db.GetEndpointParams{ID: "ep", UserID: testUser})
	assertEqual(t, "legacy destination_url", ep.DestinationUrl, "http://localhost/otto")

	if _, err := store.RemoveDestination(ctx, testUser, dests["otto"].ID); err != nil {
		t.Fatalf("remove primary: %v", err)
	}
	ep, _ = store.GetEndpoint(ctx, db.GetEndpointParams{ID: "ep", UserID: testUser})
	assertEqual(t, "legacy destination_url follows new primary", ep.DestinationUrl, "http://localhost/schoolboy")

	// Legacy single-destination update goes to the primary
	if _, err := store.UpdateEndpointAndPrimary(ctx, db.UpdateEndpointParams{ID: "ep", UserID: testUser, DestinationUrl: sql.NullString{String: "http://localhost/new", Valid: true}}); err != nil {
		t.Fatalf("update endpoint: %v", err)
	}
	d, _ := store.GetDestination(ctx, dests["schoolboy"].ID)
	assertEqual(t, "primary url", d.Url, "http://localhost/new")
}

func TestReplay(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	dests := createTestEndpoint(t, store, "ep", "otto", "schoolboy")
	receiveWebhook(t, store, "ep", "w1")
	apply(t, store, deliveryFor(t, store, "w1", dests["otto"].ID).ID, success)
	apply(t, store, deliveryFor(t, store, "w1", dests["schoolboy"].ID).ID, permanent)
	assertEqual(t, "before replay", webhookStatus(t, store, "w1"), db.StatusFailed)

	// Replay only the failed destination: otto is not re-delivered
	wh, err := store.ReplayWebhook(ctx, testUser, "w1", dests["schoolboy"].ID)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	assertEqual(t, "webhook after targeted replay", wh.Status, db.StatusPending)
	assertEqual(t, "dispatchable", dispatchable(t, store), []string{"w1→schoolboy"})
	assertEqual(t, "attempts reset", deliveryFor(t, store, "w1", dests["schoolboy"].ID).Attempts, 0)

	// Replay all
	if _, err := store.ReplayWebhook(ctx, testUser, "w1", ""); err != nil {
		t.Fatalf("replay all: %v", err)
	}
	assertEqual(t, "dispatchable", dispatchable(t, store), []string{"w1→otto", "w1→schoolboy"})

	// Replay all never reaches a destination that was added after the webhook...
	later, _ := store.AddDestination(ctx, testUser, "ep", db.DestinationSpec{Name: "later", URL: "http://localhost/later", Enabled: true})
	store.ReplayWebhook(ctx, testUser, "w1", "")
	assertEqual(t, "replay all skips later destination", len(dispatchable(t, store)), 2)
	// ...but it can be targeted explicitly
	if _, err := store.ReplayWebhook(ctx, testUser, "w1", later.ID); err != nil {
		t.Fatalf("replay to later destination: %v", err)
	}
	assertEqual(t, "dispatchable", dispatchable(t, store), []string{"w1→otto", "w1→schoolboy", "w1→later"})

	// A destination of another endpoint is rejected
	other := createTestEndpoint(t, store, "other", "x")
	if _, err := store.ReplayWebhook(ctx, testUser, "w1", other["x"].ID); !errors.Is(err, db.ErrDestinationMismatch) {
		t.Errorf("foreign destination error = %v", err)
	}
}

func TestDeadLetterPerDestination(t *testing.T) {
	store, conn := newTestStore(t)
	ctx := context.Background()
	dests := createTestEndpoint(t, store, "ep", "otto", "schoolboy")
	receiveWebhook(t, store, "ep", "old")
	receiveWebhook(t, store, "ep", "new")
	apply(t, store, deliveryFor(t, store, "old", dests["otto"].ID).ID, success)
	apply(t, store, deliveryFor(t, store, "old", dests["schoolboy"].ID).ID, transient)
	if _, err := conn.Exec("UPDATE webhooks SET received_at = datetime('now', '-8 days') WHERE id = 'old'"); err != nil {
		t.Fatal(err)
	}

	count, err := store.MarkDeadLetters(ctx)
	if err != nil {
		t.Fatalf("mark dead letters: %v", err)
	}
	assertEqual(t, "dead lettered", count, 1)
	assertEqual(t, "otto", deliveryFor(t, store, "old", dests["otto"].ID).Status, db.StatusDelivered)
	assertEqual(t, "schoolboy", deliveryFor(t, store, "old", dests["schoolboy"].ID).Status, db.StatusDeadLetter)
	assertEqual(t, "old webhook", webhookStatus(t, store, "old"), db.StatusDeadLetter)
	assertEqual(t, "new webhook", webhookStatus(t, store, "new"), db.StatusPending)

	// The notification names the destination that failed
	rows, err := store.GetUnnotifiedDeadLetterDeliveries(ctx, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("unnotified dead letters = %v, %v", rows, err)
	}
	assertEqual(t, "destination name", rows[0].DestinationName, "schoolboy")
	assertEqual(t, "endpoint name", rows[0].EndpointName, "ep")
}

func TestResolveAckFromLegacyHub(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	dests := createTestEndpoint(t, store, "ep", "otto", "schoolboy")
	receiveWebhook(t, store, "ep", "w1")

	// A hub that predates fan-out acks by webhook ID only: primary destination
	id, err := store.ResolveAckDelivery(ctx, "w1", "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	assertEqual(t, "legacy ack resolves to primary", id, deliveryFor(t, store, "w1", dests["otto"].ID).ID)

	// A fan-out hub names the delivery
	id, _ = store.ResolveAckDelivery(ctx, "w1", "explicit")
	assertEqual(t, "explicit delivery id", id, "explicit")

	apply(t, store, deliveryFor(t, store, "w1", dests["otto"].ID).ID, success)
	apply(t, store, deliveryFor(t, store, "w1", dests["schoolboy"].ID).ID, success)
	if _, err := store.ResolveAckDelivery(ctx, "w1", ""); !errors.Is(err, db.ErrDeliveryNotPending) {
		t.Errorf("resolve with nothing pending = %v", err)
	}
}

// Regression: queries mixing sqlc.narg with bare "?" placeholders got the wrong
// parameter numbers under SQLite ("not enough args to execute query").
func TestQueriesWithOptionalParams(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	createTestEndpoint(t, store, "ep", "otto")
	receiveWebhook(t, store, "ep", "w1")
	receiveWebhook(t, store, "ep", "w2")

	all, err := store.ListWebhooks(ctx, db.ListWebhooksParams{UserID: testUser, Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("list webhooks: %v", err)
	}
	assertEqual(t, "paged webhooks", len(all), 1)

	filtered, err := store.ListWebhooks(ctx, db.ListWebhooksParams{UserID: testUser, EndpointID: "ep", Status: db.StatusPending, Limit: 10})
	if err != nil {
		t.Fatalf("list webhooks filtered: %v", err)
	}
	assertEqual(t, "filtered webhooks", len(filtered), 2)

	ep, err := store.UpdateEndpoint(ctx, db.UpdateEndpointParams{ID: "ep", UserID: testUser, Muted: sql.NullInt64{Int64: 1, Valid: true}})
	if err != nil {
		t.Fatalf("update endpoint: %v", err)
	}
	assertEqual(t, "muted", ep.Muted, 1)
	assertEqual(t, "name untouched", ep.Name, "ep")
}

// Destinations and replay are scoped to the endpoint's owner.
func TestDestinationsAreUserScoped(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	dests := createTestEndpoint(t, store, "ep", "otto", "schoolboy")
	receiveWebhook(t, store, "ep", "w1")
	apply(t, store, deliveryFor(t, store, "w1", dests["otto"].ID).ID, success)

	const intruder = "user-2"
	if _, err := store.AddDestination(ctx, intruder, "ep", db.DestinationSpec{Name: "evil", URL: "http://evil", Enabled: true}); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("add to another user's endpoint = %v, want not found", err)
	}
	url := "http://evil"
	if _, err := store.UpdateDestination(ctx, intruder, dests["otto"].ID, nil, &url, nil); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("update another user's destination = %v, want not found", err)
	}
	if _, err := store.RemoveDestination(ctx, intruder, dests["schoolboy"].ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("remove another user's destination = %v, want not found", err)
	}
	if _, err := store.ReplayWebhook(ctx, intruder, "w1", ""); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("replay another user's webhook = %v, want not found", err)
	}

	d, _ := store.GetDestination(ctx, dests["otto"].ID)
	assertEqual(t, "url untouched", d.Url, "http://localhost/otto")
	assertEqual(t, "delivery untouched", deliveryFor(t, store, "w1", dests["otto"].ID).Status, db.StatusDelivered)
}
