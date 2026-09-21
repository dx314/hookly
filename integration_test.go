// TestIntegrationFanout (bottom of this file) is self-contained: it runs an edge,
// a hookly relay client and two destinations in-process.
//
// Integration tests for Hookly CLI authentication
//
// These tests run against the live server at hooks.dx314.com
// Run with: go test -v -run Integration
//
// Prerequisites:
// - Run 'hookly login' to authenticate first
// - Or set HOOKLY_TEST_TOKEN environment variable

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-chi/chi/v5"

	hooklyv1 "hooks.dx314.com/internal/api/hookly/v1"
	"hooks.dx314.com/internal/api/hookly/v1/hooklyv1connect"
	"hooks.dx314.com/internal/auth"
	"hooks.dx314.com/internal/cli"
	"hooks.dx314.com/internal/config"
	"hooks.dx314.com/internal/crypto"
	"hooks.dx314.com/internal/db"
	"hooks.dx314.com/internal/relay"
	"hooks.dx314.com/internal/webhook"
)

const testEdgeURL = "https://hooks.dx314.com"

// getTestCredentials loads credentials for testing.
// First checks HOOKLY_TEST_TOKEN env var, then falls back to saved credentials.
func getTestCredentials(t *testing.T) *cli.Credentials {
	t.Helper()

	// Check for env var first (useful for CI)
	if token := os.Getenv("HOOKLY_TEST_TOKEN"); token != "" {
		return &cli.Credentials{
			EdgeURL:   testEdgeURL,
			APIToken:  token,
			UserID:    os.Getenv("HOOKLY_TEST_USER_ID"),
			Username:  os.Getenv("HOOKLY_TEST_USERNAME"),
			CreatedAt: time.Now(),
		}
	}

	// Fall back to saved credentials
	mgr, err := cli.NewCredentialsManager()
	if err != nil {
		t.Fatalf("NewCredentialsManager: %v", err)
	}

	creds, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load credentials: %v", err)
	}

	if creds == nil {
		t.Skip("No credentials found. Run 'hookly login' or set HOOKLY_TEST_TOKEN")
	}

	return creds
}

func TestIntegrationListEndpoints(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	creds := getTestCredentials(t)
	client := cli.NewClient(creds.EdgeURL, creds.APIToken)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.Edge.ListEndpoints(ctx, connect.NewRequest(&hooklyv1.ListEndpointsRequest{}))
	if err != nil {
		t.Fatalf("ListEndpoints: %v", err)
	}

	t.Logf("Found %d endpoints", len(resp.Msg.Endpoints))
	for _, ep := range resp.Msg.Endpoints {
		t.Logf("  - %s (%s) -> %s", ep.Name, ep.Id, ep.DestinationUrl)
	}

	// Should have at least one endpoint (assuming test account has endpoints)
	if len(resp.Msg.Endpoints) == 0 {
		t.Log("Warning: no endpoints found - create one in the web UI to fully test")
	}
}

func TestIntegrationGetEndpoint(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	creds := getTestCredentials(t)
	client := cli.NewClient(creds.EdgeURL, creds.APIToken)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First list endpoints to get an ID
	listResp, err := client.Edge.ListEndpoints(ctx, connect.NewRequest(&hooklyv1.ListEndpointsRequest{}))
	if err != nil {
		t.Fatalf("ListEndpoints: %v", err)
	}

	if len(listResp.Msg.Endpoints) == 0 {
		t.Skip("no endpoints found - create one to test GetEndpoint")
	}

	endpointID := listResp.Msg.Endpoints[0].Id

	// Get the specific endpoint
	getResp, err := client.Edge.GetEndpoint(ctx, connect.NewRequest(&hooklyv1.GetEndpointRequest{
		Id: endpointID,
	}))
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}

	ep := getResp.Msg.Endpoint
	t.Logf("Got endpoint: %s (%s)", ep.Name, ep.Id)
	t.Logf("  Provider: %s", ep.ProviderType)
	t.Logf("  Destination: %s", ep.DestinationUrl)
	t.Logf("  Muted: %v", ep.Muted)
}

func TestIntegrationGetStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	creds := getTestCredentials(t)
	client := cli.NewClient(creds.EdgeURL, creds.APIToken)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.Edge.GetStatus(ctx, connect.NewRequest(&hooklyv1.GetStatusRequest{}))
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	status := resp.Msg.Status
	if status == nil {
		t.Fatal("status is nil")
	}

	t.Logf("Server status:")
	t.Logf("  Pending webhooks: %d", status.PendingCount)
	t.Logf("  Failed webhooks: %d", status.FailedCount)
	t.Logf("  Dead letter webhooks: %d", status.DeadLetterCount)
	t.Logf("  Home hub connected: %v", status.HomeHubConnected)
}

func TestIntegrationInvalidToken(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create client with invalid token
	client := cli.NewClient(testEdgeURL, "hk_invalid_token_12345")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := client.Edge.ListEndpoints(ctx, connect.NewRequest(&hooklyv1.ListEndpointsRequest{}))
	if err == nil {
		t.Fatal("expected error with invalid token, got nil")
	}

	t.Logf("Got expected error: %v", err)
}

func TestIntegrationNoToken(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create client with empty token
	client := cli.NewClient(testEdgeURL, "")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := client.Edge.ListEndpoints(ctx, connect.NewRequest(&hooklyv1.ListEndpointsRequest{}))
	if err == nil {
		t.Fatal("expected error with empty token, got nil")
	}

	t.Logf("Got expected error: %v", err)
}

func TestIntegrationListWebhooks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	creds := getTestCredentials(t)
	client := cli.NewClient(creds.EdgeURL, creds.APIToken)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First get an endpoint
	listResp, err := client.Edge.ListEndpoints(ctx, connect.NewRequest(&hooklyv1.ListEndpointsRequest{}))
	if err != nil {
		t.Fatalf("ListEndpoints: %v", err)
	}

	if len(listResp.Msg.Endpoints) == 0 {
		t.Skip("no endpoints found")
	}

	endpointID := listResp.Msg.Endpoints[0].Id

	// List webhooks for this endpoint
	webhooksResp, err := client.Edge.ListWebhooks(ctx, connect.NewRequest(&hooklyv1.ListWebhooksRequest{
		EndpointId: &endpointID,
	}))
	if err != nil {
		t.Fatalf("ListWebhooks: %v", err)
	}

	t.Logf("Found %d webhooks for endpoint %s", len(webhooksResp.Msg.Webhooks), endpointID)
	for i, wh := range webhooksResp.Msg.Webhooks {
		if i >= 5 {
			t.Logf("  ... and %d more", len(webhooksResp.Msg.Webhooks)-5)
			break
		}
		t.Logf("  - %s: %s (attempts: %d)", wh.Id, wh.Status, wh.Attempts)
	}
}

func TestIntegrationCredentialsPersistence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// This test verifies that credentials can be saved and loaded correctly
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	mgr, err := cli.NewCredentialsManager()
	if err != nil {
		t.Fatalf("NewCredentialsManager: %v", err)
	}

	// Create test credentials
	testCreds := &cli.Credentials{
		EdgeURL:   testEdgeURL,
		APIToken:  "hk_test_persistence_token_" + time.Now().Format("20060102150405"),
		UserID:    "test-user-id",
		Username:  "test-username",
		CreatedAt: time.Now(),
	}

	// Save
	if err := mgr.Save(testCreds); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Create a new manager (simulating a new process)
	mgr2, err := cli.NewCredentialsManager()
	if err != nil {
		t.Fatalf("NewCredentialsManager (2): %v", err)
	}

	// Load
	loaded, err := mgr2.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded == nil {
		t.Fatal("loaded credentials is nil")
	}

	// Verify all fields
	if loaded.EdgeURL != testCreds.EdgeURL {
		t.Errorf("EdgeURL: got %q, want %q", loaded.EdgeURL, testCreds.EdgeURL)
	}
	if loaded.APIToken != testCreds.APIToken {
		t.Errorf("APIToken mismatch")
	}
	if loaded.UserID != testCreds.UserID {
		t.Errorf("UserID: got %q, want %q", loaded.UserID, testCreds.UserID)
	}
	if loaded.Username != testCreds.Username {
		t.Errorf("Username: got %q, want %q", loaded.Username, testCreds.Username)
	}
}

func TestIntegrationHealthCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Simple HTTP health check - doesn't require authentication
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", testEdgeURL+"/health", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Health check request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Health check status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	t.Logf("Server health check: OK")
}

// recordingDestination is a local service that records the webhooks it accepts.
type recordingDestination struct {
	mu        sync.Mutex
	failFirst int      // respond 500 to this many requests before recovering
	requests  int      // every request, including rejected ones
	accepted  []string // bodies answered with 200, in order
	secrets   []string // X-Telegram-Bot-Api-Secret-Token of accepted requests
}

func (d *recordingDestination) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	d.mu.Lock()
	defer d.mu.Unlock()
	d.requests++
	if d.requests <= d.failFirst {
		http.Error(w, "down", http.StatusInternalServerError)
		return
	}
	d.accepted = append(d.accepted, string(body))
	d.secrets = append(d.secrets, r.Header.Get("X-Telegram-Bot-Api-Secret-Token"))
	w.WriteHeader(http.StatusOK)
}

func (d *recordingDestination) snapshot() (accepted []string, requests int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.accepted), d.requests
}

// TestIntegrationFanout sends webhooks to one Telegram endpoint with two
// destinations through a real edge (ingest + relay + dispatcher) and a real
// home-hub client. One destination returns 500 and then recovers.
func TestIntegrationFanout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Destinations on the "home network"
	healthy := &recordingDestination{}
	flaky := &recordingDestination{failFirst: 2}
	healthySrv := httptest.NewServer(healthy)
	defer healthySrv.Close()
	flakySrv := httptest.NewServer(flaky)
	defer flakySrv.Close()

	// Edge
	const userID = "user-1"
	const telegramSecret = "telegram-secret-token"

	conn, err := db.Open(ctx, filepath.Join(t.TempDir(), "edge.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer conn.Close()
	store := db.NewStore(conn)

	key, err := crypto.ParseKey("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	if err != nil {
		t.Fatalf("parse key: %v", err)
	}
	secretManager := db.NewSecretManager(key)
	encrypted, err := secretManager.EncryptSecret(telegramSecret)
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}

	const endpointID = "ep_fanout"
	if _, err := store.CreateEndpointWithDestinations(ctx, db.CreateEndpointParams{
		ID: endpointID, UserID: userID, Name: "bot", ProviderType: "telegram", SignatureSecretEncrypted: encrypted,
	}, []db.DestinationSpec{
		{Name: "otto", URL: healthySrv.URL + "/telegram", Enabled: true},
		{Name: "schoolboy", URL: "http://edge-configured.invalid/telegram", Enabled: true},
	}); err != nil {
		t.Fatalf("create endpoint: %v", err)
	}

	// The hub authenticates with a CLI bearer token owned by the endpoint's user
	tokenManager := auth.NewTokenManager(store.Queries)
	hubToken, _, err := tokenManager.GenerateToken(ctx, userID, "alex", "test-hub")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	connMgr := relay.NewConnectionManager()
	r := chi.NewRouter()
	r.Post("/h/{endpointID}", webhook.NewHandler(store, secretManager).ServeHTTP)
	relayPath, relayHandler := hooklyv1connect.NewRelayServiceHandler(relay.NewHandler(tokenManager, connMgr, store, nil))
	r.Mount(relayPath, relayHandler)

	edgeSrv := httptest.NewUnstartedServer(r)
	edgeSrv.EnableHTTP2 = true // bidi streaming needs HTTP/2
	edgeSrv.StartTLS()
	defer edgeSrv.Close()
	defer edgeSrv.CloseClientConnections()
	defer cancel() // end the hub's stream first, or Close waits for it

	go relay.NewDispatcher(store, connMgr).Run(ctx)

	// Home-hub: overrides the schoolboy destination by name, like a real hookly.yaml
	hub := relay.NewClient(&config.HooklyConfig{
		EdgeURL: edgeSrv.URL,
		Token:   hubToken,
		HubID:   "test-hub",
		Endpoints: []config.EndpointConfig{{
			ID:           endpointID,
			Destinations: map[string]string{"schoolboy": flakySrv.URL + "/telegram"},
		}},
	})
	hub.SetHTTPClient(edgeSrv.Client())
	go hub.Run(ctx)

	// Telegram posts updates; the edge must answer 200 immediately, before any delivery
	var want []string
	for i := 1; i <= 4; i++ {
		body := fmt.Sprintf(`{"update_id":%d}`, i)
		want = append(want, body)

		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, edgeSrv.URL+"/h/"+endpointID, bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", telegramSecret)
		start := time.Now()
		resp, err := edgeSrv.Client().Do(req)
		if err != nil {
			t.Fatalf("post webhook %d: %v", i, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("webhook %d: edge answered %d, want 200", i, resp.StatusCode)
		}
		if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
			t.Errorf("webhook %d: edge took %v to answer, it must not wait for delivery", i, elapsed)
		}
	}

	// Wait until both destinations have everything (flaky backs off 2s + 4s first)
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		gotHealthy, _ := healthy.snapshot()
		gotFlaky, _ := flaky.snapshot()
		if len(gotHealthy) >= len(want) && len(gotFlaky) >= len(want) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	// Give duplicates a chance to show up
	time.Sleep(2500 * time.Millisecond)

	gotHealthy, healthyRequests := healthy.snapshot()
	gotFlaky, flakyRequests := flaky.snapshot()

	// The healthy destination gets each webhook exactly once, in order, and is
	// never re-delivered to because the other one failed.
	if !slices.Equal(gotHealthy, want) {
		t.Errorf("healthy destination received %v, want %v", gotHealthy, want)
	}
	if healthyRequests != len(want) {
		t.Errorf("healthy destination got %d requests, want exactly %d", healthyRequests, len(want))
	}

	// The flaky destination catches up in order once it recovers
	if !slices.Equal(gotFlaky, want) {
		t.Errorf("flaky destination received %v, want %v", gotFlaky, want)
	}
	if flakyRequests != len(want)+flaky.failFirst {
		t.Errorf("flaky destination got %d requests, want %d (2 rejected + one per webhook)", flakyRequests, len(want)+flaky.failFirst)
	}

	// Original headers reach every destination; verification happened once, at the edge
	for _, d := range []*recordingDestination{healthy, flaky} {
		for _, secret := range d.secrets {
			if secret != telegramSecret {
				t.Errorf("destination saw secret token %q, want the original header", secret)
			}
		}
	}

	// Edge state: everything delivered, retries only counted against the flaky destination
	webhooks, err := store.ListWebhooks(ctx, db.ListWebhooksParams{UserID: userID, Limit: 100})
	if err != nil {
		t.Fatalf("list webhooks: %v", err)
	}
	if len(webhooks) != len(want) {
		t.Fatalf("edge stored %d webhooks, want %d", len(webhooks), len(want))
	}
	for _, wh := range webhooks {
		if wh.Status != db.StatusDelivered || wh.SignatureValid != 1 {
			t.Errorf("webhook %s: status %s signature_valid %d", wh.ID, wh.Status, wh.SignatureValid)
		}
		deliveries, _ := store.ListDeliveriesByWebhook(ctx, wh.ID)
		if len(deliveries) != 2 {
			t.Fatalf("webhook %s has %d deliveries, want 2", wh.ID, len(deliveries))
		}
		for _, d := range deliveries {
			if d.Status != db.StatusDelivered {
				t.Errorf("webhook %s → %s: %s", wh.ID, d.DestinationName, d.Status)
			}
			if d.DestinationName == "otto" && d.Attempts != 1 {
				t.Errorf("webhook %s → otto: %d attempts, want 1", wh.ID, d.Attempts)
			}
		}
	}
}
