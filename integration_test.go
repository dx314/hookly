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
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"

	hooklyv1 "hooks.dx314.com/internal/api/hookly/v1"
	"hooks.dx314.com/internal/cli"
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
