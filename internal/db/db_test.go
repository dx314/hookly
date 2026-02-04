package db_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"hooks.dx314.com/internal/crypto"
	"hooks.dx314.com/internal/db"
)

func TestDatabaseCreation(t *testing.T) {
	ctx := context.Background()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "hookly-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Open database (should create and migrate)
	conn, err := db.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer conn.Close()

	// Verify file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("database file not created")
	}

	// Verify queries work
	queries := db.New(conn)

	// Create an endpoint
	key, _ := crypto.ParseKey("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	sm := db.NewSecretManager(key)
	encryptedSecret, err := sm.EncryptSecret("test-secret")
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}

	endpoint, err := queries.CreateEndpoint(ctx, db.CreateEndpointParams{
		ID:                       "test-endpoint-1",
		Name:                     "Test Endpoint",
		ProviderType:             "github",
		SignatureSecretEncrypted: encryptedSecret,
		DestinationUrl:           "http://localhost:8080/hook",
	})
	if err != nil {
		t.Fatalf("create endpoint: %v", err)
	}

	if endpoint.ID != "test-endpoint-1" {
		t.Errorf("expected ID test-endpoint-1, got %s", endpoint.ID)
	}

	// Decrypt and verify secret
	decrypted, err := sm.DecryptSecret(endpoint.SignatureSecretEncrypted)
	if err != nil {
		t.Fatalf("decrypt secret: %v", err)
	}
	if decrypted != "test-secret" {
		t.Errorf("expected secret test-secret, got %s", decrypted)
	}
}

func TestEncryption(t *testing.T) {
	key, err := crypto.ParseKey("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("parse key: %v", err)
	}

	plaintext := "my-secret-webhook-key"

	ciphertext, err := crypto.Encrypt([]byte(plaintext), key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	decrypted, err := crypto.Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if string(decrypted) != plaintext {
		t.Errorf("expected %s, got %s", plaintext, string(decrypted))
	}
}
