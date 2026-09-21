package db_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"hooks.dx314.com/internal/db"
)

// productionVersion is the goose version production ran before
// multi-destination fan-out (migration 007).
const productionVersion = 6

// legacyData is what a single-destination production database holds.
const legacyData = `
INSERT INTO endpoints (id, user_id, name, provider_type, signature_secret_encrypted, destination_url, muted, created_at, updated_at) VALUES
    ('ep_telegram', 'u1', 'Otto bot', 'telegram', x'0102', 'http://192.168.50.134:8788/telegram', 0, '2026-01-01 00:00:00', '2026-02-01 00:00:00'),
    ('ep_github', 'u1', 'GitHub', 'github', x'0304', 'http://192.168.50.134:9000/gh', 1, '2026-01-02 00:00:00', '2026-01-02 00:00:00'),
    ('ep_empty', 'u2', 'No traffic', 'generic', x'05', 'http://localhost:1/', 0, '2026-01-03 00:00:00', '2026-01-03 00:00:00');

INSERT INTO webhooks (id, endpoint_id, received_at, headers, payload, signature_valid, status, attempts, last_attempt_at, delivered_at, error_message, notification_sent) VALUES
    ('wh_delivered', 'ep_telegram', datetime('now', '-3 hours'), '{"X-A":"1"}', x'7b7d', 1, 'delivered', 2, datetime('now', '-3 hours'), datetime('now', '-3 hours'), NULL, 0),
    ('wh_failed', 'ep_telegram', datetime('now', '-2 hours'), '{}', x'01', 1, 'failed', 1, datetime('now', '-2 hours'), NULL, 'HTTP 404', 1),
    ('wh_retrying', 'ep_telegram', datetime('now', '-1 hours'), '{}', x'02', 0, 'pending', 5, datetime('now', '-10 minutes'), NULL, 'HTTP 502', 0),
    ('wh_queued', 'ep_telegram', datetime('now', '-1 minutes'), '{}', x'03', 1, 'pending', 0, NULL, NULL, NULL, 0),
    ('wh_dead', 'ep_github', datetime('now', '-9 days'), '{}', x'04', 1, 'dead_letter', 170, datetime('now', '-2 days'), NULL, 'network error', 1);

INSERT INTO sessions (id, user_id, username, expires_at) VALUES ('s1', 'u1', 'alex', '2099-01-01 00:00:00');
INSERT INTO api_tokens (id, user_id, username, token_hash, name) VALUES ('t1', 'u1', 'alex', 'hash', 'cli');
`

type webhookSnapshot struct {
	ID, EndpointID, ReceivedAt, Headers, Status string
	Payload                                     []byte
	SignatureValid, Attempts, NotificationSent  int64
	LastAttemptAt, DeliveredAt, ErrorMessage    sql.NullString
}

func snapshotWebhooks(t *testing.T, conn *sql.DB) []webhookSnapshot {
	t.Helper()
	rows, err := conn.Query(`SELECT id, endpoint_id, received_at, headers, payload, signature_valid, status, attempts,
		last_attempt_at, delivered_at, error_message, notification_sent FROM webhooks ORDER BY id`)
	if err != nil {
		t.Fatalf("snapshot webhooks: %v", err)
	}
	defer rows.Close()
	var out []webhookSnapshot
	for rows.Next() {
		var w webhookSnapshot
		if err := rows.Scan(&w.ID, &w.EndpointID, &w.ReceivedAt, &w.Headers, &w.Payload, &w.SignatureValid, &w.Status,
			&w.Attempts, &w.LastAttemptAt, &w.DeliveredAt, &w.ErrorMessage, &w.NotificationSent); err != nil {
			t.Fatalf("scan webhook: %v", err)
		}
		out = append(out, w)
	}
	return out
}

func count(t *testing.T, conn *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// TestMigrateLegacyDatabase opens a database as production's single-destination
// release left it and checks that it upgrades in place without losing anything.
func TestMigrateLegacyDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")

	legacy, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("create legacy database: %v", err)
	}
	if err := db.MigrateTo(ctx, legacy, productionVersion); err != nil {
		t.Fatalf("migrate to production version: %v", err)
	}
	if _, err := legacy.Exec(legacyData); err != nil {
		t.Fatalf("insert legacy data: %v", err)
	}
	before := snapshotWebhooks(t, legacy)
	legacy.Close()

	// Upgrade in place on start
	conn, err := db.Open(ctx, path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	store := db.NewStore(conn)

	// Existing rows are untouched
	after := snapshotWebhooks(t, conn)
	assertEqual(t, "webhooks after migration", after, before)
	assertEqual(t, "endpoints", count(t, conn, "SELECT COUNT(*) FROM endpoints"), 3)
	assertEqual(t, "sessions", count(t, conn, "SELECT COUNT(*) FROM sessions"), 1)
	assertEqual(t, "api tokens", count(t, conn, "SELECT COUNT(*) FROM api_tokens"), 1)
	assertEqual(t, "legacy column kept", count(t, conn, "SELECT COUNT(*) FROM endpoints WHERE destination_url != ''"), 3)

	// Every endpoint has exactly one destination, taken from destination_url
	for id, url := range map[string]string{
		"ep_telegram": "http://192.168.50.134:8788/telegram",
		"ep_github":   "http://192.168.50.134:9000/gh",
		"ep_empty":    "http://localhost:1/",
	} {
		dests, err := store.ListDestinationsByEndpoint(ctx, id)
		if err != nil || len(dests) != 1 {
			t.Fatalf("%s destinations = %v, %v; want exactly one", id, dests, err)
		}
		assertEqual(t, id+" url", dests[0].Url, url)
		assertEqual(t, id+" name", dests[0].Name, db.DefaultDestinationName)
		assertEqual(t, id+" enabled", dests[0].Enabled, 1)
	}

	// Every webhook has exactly one delivery carrying its own state
	assertEqual(t, "deliveries", count(t, conn, "SELECT COUNT(*) FROM deliveries"), len(before))
	for _, w := range before {
		deliveries, err := store.ListDeliveriesByWebhook(ctx, w.ID)
		if err != nil || len(deliveries) != 1 {
			t.Fatalf("%s deliveries = %v, %v; want exactly one", w.ID, deliveries, err)
		}
		d := deliveries[0]
		assertEqual(t, w.ID+" status", d.Status, w.Status)
		assertEqual(t, w.ID+" attempts", d.Attempts, w.Attempts)
		assertEqual(t, w.ID+" last_attempt_at", d.LastAttemptAt, w.LastAttemptAt)
		assertEqual(t, w.ID+" delivered_at", d.DeliveredAt, w.DeliveredAt)
		assertEqual(t, w.ID+" error_message", d.ErrorMessage, w.ErrorMessage)
		assertEqual(t, w.ID+" notification_sent", d.NotificationSent, w.NotificationSent)
	}

	// In-flight retries continue in arrival order with their backoff state
	// (ep_github is muted; wh_retrying is past its 32s backoff)
	assertEqual(t, "dispatchable", dispatchable(t, store), []string{"wh_retrying→default"})
	retrying, _ := store.ListDeliveriesByWebhook(ctx, "wh_retrying")
	apply(t, store, retrying[0].ID, success)
	assertEqual(t, "next in order", dispatchable(t, store), []string{"wh_queued→default"})
	assertEqual(t, "rollup", webhookStatus(t, store, "wh_retrying"), db.StatusDelivered)

	// Already-notified failures are not notified again
	unnotified, _ := store.GetUnnotifiedDeadLetterDeliveries(ctx, 10)
	assertEqual(t, "unnotified dead letters", len(unnotified), 0)
	conn.Close()

	// Restarting is a no-op
	conn, err = db.Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	assertEqual(t, "destinations after restart", count(t, conn, "SELECT COUNT(*) FROM destinations"), 3)
	assertEqual(t, "deliveries after restart", count(t, conn, "SELECT COUNT(*) FROM deliveries"), len(before))

	// Rollback safety: rows written by an older binary (no destination / delivery
	// rows) are adopted on the next start; finished webhooks are not re-fanned out.
	if _, err := conn.Exec(`
		INSERT INTO endpoints (id, user_id, name, provider_type, signature_secret_encrypted, destination_url) VALUES ('ep_rollback', 'u1', 'r', 'generic', x'00', 'http://localhost:2/');
		INSERT INTO webhooks (id, endpoint_id, headers, payload, signature_valid, status) VALUES ('wh_rollback', 'ep_rollback', '{}', x'00', 1, 'pending');
		DELETE FROM deliveries WHERE webhook_id = 'wh_delivered';`); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	conn, err = db.Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen after rollback: %v", err)
	}
	defer conn.Close()
	assertEqual(t, "rollback endpoint destination", count(t, conn, "SELECT COUNT(*) FROM destinations WHERE endpoint_id = 'ep_rollback'"), 1)
	assertEqual(t, "rollback webhook delivery", count(t, conn, "SELECT COUNT(*) FROM deliveries WHERE webhook_id = 'wh_rollback' AND status = 'pending'"), 1)
	assertEqual(t, "finished webhook not fanned out again", count(t, conn, "SELECT COUNT(*) FROM deliveries WHERE webhook_id = 'wh_delivered'"), 0)
}
