package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"connectrpc.com/connect"

	"hooks.dx314.com/internal/auth"
)

// AuthInterceptor validates session cookies or Bearer tokens for ConnectRPC handlers.
type AuthInterceptor struct {
	sessions *auth.SessionManager
	tokens   *auth.TokenManager
}

// NewAuthInterceptor creates a new auth interceptor.
func NewAuthInterceptor(sessions *auth.SessionManager, tokens *auth.TokenManager) *AuthInterceptor {
	return &AuthInterceptor{sessions: sessions, tokens: tokens}
}

// WrapUnary implements connect.Interceptor.
func (i *AuthInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		ctx, err := i.authenticate(ctx, req.Header())
		if err != nil {
			return nil, connect.NewError(connect.CodeUnauthenticated, err)
		}
		return next(ctx, req)
	}
}

// WrapStreamingClient implements connect.Interceptor.
func (i *AuthInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// WrapStreamingHandler implements connect.Interceptor.
func (i *AuthInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		ctx, err := i.authenticate(ctx, conn.RequestHeader())
		if err != nil {
			return connect.NewError(connect.CodeUnauthenticated, err)
		}
		return next(ctx, conn)
	}
}

// authenticate extracts and validates credentials from headers.
// It checks Bearer token first (for CLI), then falls back to session cookie (for web UI).
func (i *AuthInterceptor) authenticate(ctx context.Context, headers http.Header) (context.Context, error) {
	// Check for Bearer token first (CLI authentication)
	authHeader := headers.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		return i.authenticateWithToken(ctx, token)
	}

	// Fall back to cookie authentication (web UI)
	return i.authenticateWithCookie(ctx, headers)
}

// authenticateWithToken validates an API token.
func (i *AuthInterceptor) authenticateWithToken(ctx context.Context, token string) (context.Context, error) {
	if i.tokens == nil {
		return nil, errors.New("token authentication not configured")
	}

	apiToken, err := i.tokens.ValidateToken(ctx, token)
	if err != nil {
		if errors.Is(err, auth.ErrTokenNotFound) || errors.Is(err, auth.ErrTokenRevoked) || errors.Is(err, auth.ErrInvalidToken) {
			return nil, errors.New("invalid or expired token")
		}
		slog.Error("auth interceptor: failed to validate token", "error", err)
		return nil, errors.New("authentication failed")
	}

	// Convert token info to session for context compatibility
	session := &auth.Session{
		ID:       apiToken.ID,
		UserID:   apiToken.UserID,
		Username: apiToken.Username,
	}

	return auth.ContextWithSession(ctx, session), nil
}

// authenticateWithCookie validates a session cookie.
func (i *AuthInterceptor) authenticateWithCookie(ctx context.Context, headers http.Header) (context.Context, error) {
	// Parse cookie header
	cookieHeader := headers.Get("Cookie")
	if cookieHeader == "" {
		return nil, errors.New("missing session cookie")
	}

	// Parse cookies from header
	request := &http.Request{Header: http.Header{"Cookie": {cookieHeader}}}
	cookie, err := request.Cookie(auth.SessionCookieName)
	if err != nil {
		return nil, errors.New("missing session cookie")
	}

	// Validate session
	session, err := i.sessions.GetSession(ctx, cookie.Value)
	if err != nil {
		slog.Error("auth interceptor: failed to get session", "error", err)
		return nil, errors.New("invalid session")
	}
	if session == nil {
		return nil, errors.New("session not found")
	}

	// Add session to context
	return auth.ContextWithSession(ctx, session), nil
}
