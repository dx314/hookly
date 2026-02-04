package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"

	"hooks.dx314.com/internal/db"
)

const (
	// TokenPrefix is the prefix for API tokens.
	TokenPrefix = "hk_"
	// TokenByteLength is the length of the random bytes in a token.
	TokenByteLength = 32
)

var (
	ErrInvalidToken = errors.New("invalid token format")
	ErrTokenRevoked = errors.New("token has been revoked")
	ErrTokenNotFound = errors.New("token not found")
)

// TokenManager handles API token operations.
type TokenManager struct {
	queries *db.Queries
}

// NewTokenManager creates a new TokenManager.
func NewTokenManager(queries *db.Queries) *TokenManager {
	return &TokenManager{queries: queries}
}

// GenerateToken creates a new API token and stores its hash.
// Returns the plaintext token (which should be shown to the user once) and the database record.
func (m *TokenManager) GenerateToken(ctx context.Context, userID, username, name string) (string, *db.ApiToken, error) {
	// Generate random bytes for token
	tokenBytes := make([]byte, TokenByteLength)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", nil, fmt.Errorf("generate token bytes: %w", err)
	}

	// Create token with prefix
	plaintext := TokenPrefix + base64.URLEncoding.EncodeToString(tokenBytes)

	// Hash the token for storage
	hash := hashToken(plaintext)

	// Generate ID for the token record
	id, err := gonanoid.New()
	if err != nil {
		return "", nil, fmt.Errorf("generate token id: %w", err)
	}

	// Store in database
	token, err := m.queries.CreateAPIToken(ctx, db.CreateAPITokenParams{
		ID:        id,
		UserID:    userID,
		Username:  username,
		TokenHash: hash,
		Name:      name,
	})
	if err != nil {
		return "", nil, fmt.Errorf("create token: %w", err)
	}

	return plaintext, &token, nil
}

// ValidateToken checks if a token is valid and returns the associated user info.
// Also updates the last_used_at timestamp.
func (m *TokenManager) ValidateToken(ctx context.Context, plaintext string) (*db.ApiToken, error) {
	if !strings.HasPrefix(plaintext, TokenPrefix) {
		return nil, ErrInvalidToken
	}

	hash := hashToken(plaintext)

	token, err := m.queries.GetAPITokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTokenNotFound
		}
		return nil, fmt.Errorf("get token: %w", err)
	}

	if token.Revoked != 0 {
		return nil, ErrTokenRevoked
	}

	// Update last used (fire-and-forget, don't fail on error)
	go func() {
		_ = m.queries.UpdateAPITokenLastUsed(context.Background(), token.ID)
	}()

	return &token, nil
}

// RevokeToken revokes a specific token by ID.
func (m *TokenManager) RevokeToken(ctx context.Context, tokenID string) error {
	return m.queries.RevokeAPIToken(ctx, tokenID)
}

// RevokeAllUserTokens revokes all tokens for a user.
func (m *TokenManager) RevokeAllUserTokens(ctx context.Context, userID string) error {
	return m.queries.RevokeAllUserAPITokens(ctx, userID)
}

// GetUserTokens returns all tokens for a user.
func (m *TokenManager) GetUserTokens(ctx context.Context, userID string) ([]db.ApiToken, error) {
	return m.queries.GetAPITokensByUser(ctx, userID)
}

// hashToken creates a SHA256 hash of the plaintext token.
func hashToken(plaintext string) string {
	h := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(h[:])
}

// TokenInfo provides user info from a validated token for use in context.
type TokenInfo struct {
	TokenID  string
	UserID   string
	Username string
}

// ToSession converts TokenInfo to a Session for compatibility with existing code.
func (t *TokenInfo) ToSession() *Session {
	return &Session{
		ID:       t.TokenID,
		UserID:   t.UserID,
		Username: t.Username,
	}
}
