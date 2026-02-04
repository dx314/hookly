package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Handlers provides HTTP handlers for authentication.
type Handlers struct {
	github     *GitHubClient
	sessions   *SessionManager
	authorizer *Authorizer
}

// NewHandlers creates a new authentication handlers.
func NewHandlers(github *GitHubClient, sessions *SessionManager, authorizer *Authorizer) *Handlers {
	return &Handlers{
		github:     github,
		sessions:   sessions,
		authorizer: authorizer,
	}
}

// Login redirects to GitHub for OAuth.
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	state, err := GenerateState()
	if err != nil {
		slog.Error("failed to generate state", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.sessions.SetStateCookie(w, state)
	http.Redirect(w, r, h.github.GetAuthURL(state), http.StatusFound)
}

// Callback handles the OAuth callback from GitHub.
func (h *Handlers) Callback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Validate state
	state := r.URL.Query().Get("state")
	if !ValidateState(r, state) {
		slog.Warn("invalid OAuth state")
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}
	h.sessions.ClearStateCookie(w)

	// Check for error from GitHub
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		errDesc := r.URL.Query().Get("error_description")
		slog.Warn("OAuth error from GitHub", "error", errMsg, "description", errDesc)
		http.Error(w, "Authorization denied: "+errDesc, http.StatusForbidden)
		return
	}

	// Exchange code for token
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing code", http.StatusBadRequest)
		return
	}

	token, err := h.github.ExchangeCode(ctx, code)
	if err != nil {
		slog.Error("failed to exchange code", "error", err)
		http.Error(w, "Failed to authenticate", http.StatusInternalServerError)
		return
	}

	// Get user info
	user, err := h.github.GetUser(ctx, token.AccessToken)
	if err != nil {
		slog.Error("failed to get user", "error", err)
		http.Error(w, "Failed to get user info", http.StatusInternalServerError)
		return
	}

	// Check authorization
	if !h.authorizer.IsAuthorized(ctx, user.Login, token.AccessToken) {
		slog.Warn("user not authorized", "username", user.Login)
		http.Error(w, "You are not authorized to access this application", http.StatusForbidden)
		return
	}

	// Create session
	session, err := h.sessions.CreateSession(ctx, user)
	if err != nil {
		slog.Error("failed to create session", "error", err)
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	h.sessions.SetSessionCookie(w, session)
	slog.Info("user logged in", "username", user.Login, "user_id", user.ID)

	// Redirect to home
	http.Redirect(w, r, "/", http.StatusFound)
}

// Logout clears the session and redirects to home.
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	session, _ := h.sessions.GetSessionFromRequest(r)
	if session != nil {
		if err := h.sessions.DeleteSession(r.Context(), session.ID); err != nil {
			slog.Error("failed to delete session", "error", err)
		}
		slog.Info("user logged out", "username", session.Username)
	}

	h.sessions.ClearSessionCookie(w)
	http.Redirect(w, r, "/", http.StatusFound)
}

// Me returns the current user information.
func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	session, err := h.sessions.GetSessionFromRequest(r)
	if err != nil {
		slog.Error("failed to get session", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if session == nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"user_id":    session.UserID,
		"username":   session.Username,
		"avatar_url": session.AvatarURL,
	})
}
