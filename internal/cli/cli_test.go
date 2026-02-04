package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCredentialsManager(t *testing.T) {
	// Use a temp directory for testing
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	mgr, err := NewCredentialsManager()
	if err != nil {
		t.Fatalf("NewCredentialsManager: %v", err)
	}

	// Test that loading non-existent credentials returns nil
	creds, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load (empty): %v", err)
	}
	if creds != nil {
		t.Errorf("expected nil credentials, got %+v", creds)
	}

	// Test saving credentials
	testCreds := &Credentials{
		EdgeURL:   "https://hooks.dx314.com",
		APIToken:  "hk_test_token_12345",
		UserID:    "12345",
		Username:  "testuser",
		CreatedAt: time.Now(),
	}

	if err := mgr.Save(testCreds); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file was created
	path := filepath.Join(tempDir, ConfigDir, CredentialsFile)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("credentials file not created at %s", path)
	}

	// Test loading credentials
	loaded, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected credentials, got nil")
	}

	if loaded.EdgeURL != testCreds.EdgeURL {
		t.Errorf("EdgeURL: got %q, want %q", loaded.EdgeURL, testCreds.EdgeURL)
	}
	if loaded.APIToken != testCreds.APIToken {
		t.Errorf("APIToken: got %q, want %q", loaded.APIToken, testCreds.APIToken)
	}
	if loaded.UserID != testCreds.UserID {
		t.Errorf("UserID: got %q, want %q", loaded.UserID, testCreds.UserID)
	}
	if loaded.Username != testCreds.Username {
		t.Errorf("Username: got %q, want %q", loaded.Username, testCreds.Username)
	}

	// Test deleting credentials
	if err := mgr.Delete(); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify credentials are gone
	creds, err = mgr.Load()
	if err != nil {
		t.Fatalf("Load (after delete): %v", err)
	}
	if creds != nil {
		t.Errorf("expected nil credentials after delete, got %+v", creds)
	}
}

func TestCredentialsEncryption(t *testing.T) {
	// Use a temp directory for testing
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	mgr, err := NewCredentialsManager()
	if err != nil {
		t.Fatalf("NewCredentialsManager: %v", err)
	}

	// Save credentials with a token
	testToken := "hk_sensitive_token_that_should_be_encrypted"
	testCreds := &Credentials{
		EdgeURL:   "https://hooks.dx314.com",
		APIToken:  testToken,
		UserID:    "12345",
		Username:  "testuser",
		CreatedAt: time.Now(),
	}

	if err := mgr.Save(testCreds); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Read the raw file and verify token is not in plaintext
	path := filepath.Join(tempDir, ConfigDir, CredentialsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// The token should NOT appear in plaintext
	if string(data) != "" && contains(string(data), testToken) {
		t.Error("token appears in plaintext in credentials file")
	}

	// But we should be able to load it back
	loaded, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.APIToken != testToken {
		t.Errorf("loaded token: got %q, want %q", loaded.APIToken, testToken)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && (s[:len(substr)] == substr || contains(s[1:], substr)))
}

func TestDeriveKey(t *testing.T) {
	key1, err := deriveKey()
	if err != nil {
		t.Fatalf("deriveKey: %v", err)
	}

	if len(key1) != 32 {
		t.Errorf("key length: got %d, want 32", len(key1))
	}

	// Key should be deterministic on the same machine
	key2, err := deriveKey()
	if err != nil {
		t.Fatalf("deriveKey (second): %v", err)
	}

	if string(key1) != string(key2) {
		t.Error("deriveKey returned different keys on same machine")
	}
}

// Integration tests - these require the live server
// Run with: go test -v -run Integration -tags=integration

func TestIntegrationClient(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Check for credentials
	mgr, err := NewCredentialsManager()
	if err != nil {
		t.Fatalf("NewCredentialsManager: %v", err)
	}

	creds, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if creds == nil {
		t.Skip("no credentials found - run 'hookly login' first")
	}

	// Create client
	client := NewClient(creds.EdgeURL, creds.APIToken)
	if client == nil {
		t.Fatal("NewClient returned nil")
	}

	// Test listing endpoints
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.Edge.ListEndpoints(ctx, nil)
	if err != nil {
		t.Fatalf("ListEndpoints: %v", err)
	}

	t.Logf("Found %d endpoints", len(resp.Msg.Endpoints))
	for _, ep := range resp.Msg.Endpoints {
		t.Logf("  - %s (%s)", ep.Name, ep.Id)
	}
}
