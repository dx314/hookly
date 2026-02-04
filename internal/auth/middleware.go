package auth

import (
	"context"
	"log/slog"
	"net/http"
)

type contextKey string

const sessionContextKey contextKey = "session"

// GetSessionFromContext retrieves the session from the context.
func GetSessionFromContext(ctx context.Context) *Session {
	session, _ := ctx.Value(sessionContextKey).(*Session)
	return session
}

// ContextWithSession returns a new context with the session.
func ContextWithSession(ctx context.Context, session *Session) context.Context {
	return context.WithValue(ctx, sessionContextKey, session)
}

// RequireAuth returns middleware that requires authentication.
func RequireAuth(sessions *SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session, err := sessions.GetSessionFromRequest(r)
			if err != nil {
				slog.Error("auth middleware: failed to get session", "error", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			if session == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Add session to context
			ctx := context.WithValue(r.Context(), sessionContextKey, session)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuth returns middleware that loads session if present but doesn't require it.
func OptionalAuth(sessions *SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session, _ := sessions.GetSessionFromRequest(r)
			if session != nil {
				ctx := context.WithValue(r.Context(), sessionContextKey, session)
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	}
}
