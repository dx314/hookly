package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"time"

	"hooks.dx314.com/internal/db"
)

const (
	// SessionCookieName is the name of the session cookie.
	SessionCookieName = "hookly_session"
	// StateCookieName is the name of the OAuth state cookie.
	StateCookieName = "hookly_oauth_state"
	// SessionDuration is how long sessions last.
	SessionDuration = 7 * 24 * time.Hour
	// StateDuration is how long OAuth state is valid.
	StateDuration = 10 * time.Minute
)

// Session represents an authenticated user session.
type Session struct {
	ID        string
	UserID    string
	Username  string
	AvatarURL string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// SessionManager handles session creation and validation.
type SessionManager struct {
	queries  *db.Queries
	secure   bool // Use secure cookies
	basePath string
}

// NewSessionManager creates a new session manager.
func NewSessionManager(queries *db.Queries, secure bool, basePath string) *SessionManager {
	return &SessionManager{
		queries:  queries,
		secure:   secure,
		basePath: basePath,
	}
}

// generateToken generates a random token.
func generateToken(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// CreateSession creates a new session for the user.
func (m *SessionManager) CreateSession(ctx context.Context, user *GitHubUser) (*Session, error) {
	sessionID, err := generateToken(32)
	if err != nil {
		return nil, fmt.Errorf("generate session id: %w", err)
	}

	avatarURL := sql.NullString{}
	if user.AvatarURL != "" {
		avatarURL = sql.NullString{String: user.AvatarURL, Valid: true}
	}

	dbSession, err := m.queries.CreateSession(ctx, db.CreateSessionParams{
		ID:        sessionID,
		UserID:    fmt.Sprintf("%d", user.ID),
		Username:  user.Login,
		AvatarUrl: avatarURL,
	})
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	expiresAt, _ := time.Parse("2006-01-02 15:04:05", dbSession.ExpiresAt)

	return &Session{
		ID:        dbSession.ID,
		UserID:    dbSession.UserID,
		Username:  dbSession.Username,
		AvatarURL: avatarURL.String,
		ExpiresAt: expiresAt,
	}, nil
}

// GetSession retrieves a session by ID.
func (m *SessionManager) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	dbSession, err := m.queries.GetSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get session: %w", err)
	}

	expiresAt, _ := time.Parse("2006-01-02 15:04:05", dbSession.ExpiresAt)

	return &Session{
		ID:        dbSession.ID,
		UserID:    dbSession.UserID,
		Username:  dbSession.Username,
		AvatarURL: dbSession.AvatarUrl.String,
		ExpiresAt: expiresAt,
	}, nil
}

// DeleteSession deletes a session.
func (m *SessionManager) DeleteSession(ctx context.Context, sessionID string) error {
	return m.queries.DeleteSession(ctx, sessionID)
}

// CleanupExpiredSessions removes all expired sessions.
func (m *SessionManager) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	return m.queries.DeleteExpiredSessions(ctx)
}

// SetSessionCookie sets the session cookie on the response.
func (m *SessionManager) SetSessionCookie(w http.ResponseWriter, session *Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    session.ID,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie clears the session cookie.
func (m *SessionManager) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// GetSessionFromRequest extracts the session from the request cookie.
func (m *SessionManager) GetSessionFromRequest(r *http.Request) (*Session, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			return nil, nil
		}
		return nil, err
	}

	return m.GetSession(r.Context(), cookie.Value)
}

// GenerateState generates a random state for CSRF protection.
func GenerateState() (string, error) {
	return generateToken(16)
}

// SetStateCookie sets the OAuth state cookie.
func (m *SessionManager) SetStateCookie(w http.ResponseWriter, state string) {
	http.SetCookie(w, &http.Cookie{
		Name:     StateCookieName,
		Value:    state,
		Path:     "/auth/callback",
		MaxAge:   int(StateDuration.Seconds()),
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ValidateState checks if the state matches the cookie.
func ValidateState(r *http.Request, state string) bool {
	cookie, err := r.Cookie(StateCookieName)
	if err != nil {
		return false
	}
	return cookie.Value == state
}

// ClearStateCookie clears the OAuth state cookie.
func (m *SessionManager) ClearStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     StateCookieName,
		Value:    "",
		Path:     "/auth/callback",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
	})
}
