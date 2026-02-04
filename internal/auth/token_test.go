package auth

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"hooks.dx314.com/internal/db"
)

func setupTestDB(t *testing.T) (*db.Queries, func()) {
	t.Helper()

	conn, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	// Create schema
	schema := `
		CREATE TABLE api_tokens (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			username TEXT NOT NULL,
			token_hash TEXT NOT NULL,
			name TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			last_used_at TEXT,
			revoked INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX idx_api_tokens_hash ON api_tokens(token_hash);
	`
	if _, err := conn.Exec(schema); err != nil {
		conn.Close()
		t.Fatalf("create schema: %v", err)
	}

	queries := db.New(conn)
	cleanup := func() { conn.Close() }

	return queries, cleanup
}

func TestTokenGeneration(t *testing.T) {
	queries, cleanup := setupTestDB(t)
	defer cleanup()

	mgr := NewTokenManager(queries)
	ctx := context.Background()

	// Generate a token
	plaintext, token, err := mgr.GenerateToken(ctx, "12345", "testuser", "CLI - testhost")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// Token should have the correct prefix
	if len(plaintext) < len(TokenPrefix) || plaintext[:len(TokenPrefix)] != TokenPrefix {
		t.Errorf("token should start with %q, got %q", TokenPrefix, plaintext[:min(len(plaintext), 10)])
	}

	// Token record should be created
	if token == nil {
		t.Fatal("token record is nil")
	}
	if token.UserID != "12345" {
		t.Errorf("UserID: got %q, want %q", token.UserID, "12345")
	}
	if token.Username != "testuser" {
		t.Errorf("Username: got %q, want %q", token.Username, "testuser")
	}
	if token.Name != "CLI - testhost" {
		t.Errorf("Name: got %q, want %q", token.Name, "CLI - testhost")
	}
	if token.Revoked != 0 {
		t.Errorf("Revoked: got %d, want 0", token.Revoked)
	}

	// The hash should be stored, not the plaintext
	if token.TokenHash == plaintext {
		t.Error("token hash should not equal plaintext")
	}
}

func TestTokenValidation(t *testing.T) {
	queries, cleanup := setupTestDB(t)
	defer cleanup()

	mgr := NewTokenManager(queries)
	ctx := context.Background()

	// Generate a token
	plaintext, _, err := mgr.GenerateToken(ctx, "12345", "testuser", "CLI - testhost")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// Validate the token
	validated, err := mgr.ValidateToken(ctx, plaintext)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if validated == nil {
		t.Fatal("validated token is nil")
	}
	if validated.UserID != "12345" {
		t.Errorf("UserID: got %q, want %q", validated.UserID, "12345")
	}

	// Invalid token should fail
	_, err = mgr.ValidateToken(ctx, "hk_invalid_token")
	if err != ErrTokenNotFound {
		t.Errorf("expected ErrTokenNotFound, got %v", err)
	}

	// Token without prefix should fail
	_, err = mgr.ValidateToken(ctx, "invalid_no_prefix")
	if err != ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestTokenRevocation(t *testing.T) {
	queries, cleanup := setupTestDB(t)
	defer cleanup()

	mgr := NewTokenManager(queries)
	ctx := context.Background()

	// Generate a token
	plaintext, token, err := mgr.GenerateToken(ctx, "12345", "testuser", "CLI - testhost")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// Revoke the token
	if err := mgr.RevokeToken(ctx, token.ID); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}

	// Token should no longer validate
	_, err = mgr.ValidateToken(ctx, plaintext)
	if err != ErrTokenNotFound {
		t.Errorf("expected ErrTokenNotFound for revoked token, got %v", err)
	}
}

func TestRevokeAllUserTokens(t *testing.T) {
	queries, cleanup := setupTestDB(t)
	defer cleanup()

	mgr := NewTokenManager(queries)
	ctx := context.Background()

	// Generate multiple tokens for the same user
	token1, _, err := mgr.GenerateToken(ctx, "12345", "testuser", "CLI - host1")
	if err != nil {
		t.Fatalf("GenerateToken 1: %v", err)
	}
	token2, _, err := mgr.GenerateToken(ctx, "12345", "testuser", "CLI - host2")
	if err != nil {
		t.Fatalf("GenerateToken 2: %v", err)
	}

	// Generate a token for a different user
	token3, _, err := mgr.GenerateToken(ctx, "67890", "otheruser", "CLI - host3")
	if err != nil {
		t.Fatalf("GenerateToken 3: %v", err)
	}

	// Revoke all tokens for user 12345
	if err := mgr.RevokeAllUserTokens(ctx, "12345"); err != nil {
		t.Fatalf("RevokeAllUserTokens: %v", err)
	}

	// User 12345's tokens should be invalid
	_, err = mgr.ValidateToken(ctx, token1)
	if err != ErrTokenNotFound {
		t.Errorf("token1: expected ErrTokenNotFound, got %v", err)
	}
	_, err = mgr.ValidateToken(ctx, token2)
	if err != ErrTokenNotFound {
		t.Errorf("token2: expected ErrTokenNotFound, got %v", err)
	}

	// Other user's token should still work
	_, err = mgr.ValidateToken(ctx, token3)
	if err != nil {
		t.Errorf("token3: expected valid, got %v", err)
	}
}

func TestGetUserTokens(t *testing.T) {
	queries, cleanup := setupTestDB(t)
	defer cleanup()

	mgr := NewTokenManager(queries)
	ctx := context.Background()

	// Generate multiple tokens
	_, _, _ = mgr.GenerateToken(ctx, "12345", "testuser", "CLI - host1")
	_, _, _ = mgr.GenerateToken(ctx, "12345", "testuser", "CLI - host2")
	_, _, _ = mgr.GenerateToken(ctx, "67890", "otheruser", "CLI - host3")

	// Get tokens for user 12345
	tokens, err := mgr.GetUserTokens(ctx, "12345")
	if err != nil {
		t.Fatalf("GetUserTokens: %v", err)
	}

	if len(tokens) != 2 {
		t.Errorf("expected 2 tokens, got %d", len(tokens))
	}

	for _, tok := range tokens {
		if tok.UserID != "12345" {
			t.Errorf("unexpected user_id: %s", tok.UserID)
		}
	}
}

func TestHashToken(t *testing.T) {
	// Same input should produce same hash
	hash1 := hashToken("hk_test_token")
	hash2 := hashToken("hk_test_token")

	if hash1 != hash2 {
		t.Error("hashToken should be deterministic")
	}

	// Different input should produce different hash
	hash3 := hashToken("hk_different_token")
	if hash1 == hash3 {
		t.Error("different tokens should produce different hashes")
	}

	// Hash should be hex-encoded SHA256 (64 chars)
	if len(hash1) != 64 {
		t.Errorf("hash length: got %d, want 64", len(hash1))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
