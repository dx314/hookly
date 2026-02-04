package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"

	"hooks.dx314.com/internal/auth"
)

// AuthInterceptor validates session cookies for ConnectRPC handlers.
type AuthInterceptor struct {
	sessions *auth.SessionManager
}

// NewAuthInterceptor creates a new auth interceptor.
func NewAuthInterceptor(sessions *auth.SessionManager) *AuthInterceptor {
	return &AuthInterceptor{sessions: sessions}
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

// authenticate extracts and validates the session from the Cookie header.
func (i *AuthInterceptor) authenticate(ctx context.Context, headers http.Header) (context.Context, error) {
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
