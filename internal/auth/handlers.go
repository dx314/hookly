package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// Handlers provides HTTP handlers for authentication.
type Handlers struct {
	github     *GitHubClient
	sessions   *SessionManager
	authorizer *Authorizer
	tokens     *TokenManager
}

// NewHandlers creates new authentication handlers.
func NewHandlers(github *GitHubClient, sessions *SessionManager, authorizer *Authorizer, tokens *TokenManager) *Handlers {
	return &Handlers{
		github:     github,
		sessions:   sessions,
		authorizer: authorizer,
		tokens:     tokens,
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

// CLILogin initiates OAuth flow for CLI authentication.
// The CLI passes its callback port in the state parameter.
// GET /auth/cli?port=12345&state=xxx
func (h *Handlers) CLILogin(w http.ResponseWriter, r *http.Request) {
	port := r.URL.Query().Get("port")
	if port == "" {
		http.Error(w, "Missing port parameter", http.StatusBadRequest)
		return
	}

	clientState := r.URL.Query().Get("state")
	if clientState == "" {
		http.Error(w, "Missing state parameter", http.StatusBadRequest)
		return
	}

	// Generate server-side state for CSRF protection
	serverState, err := GenerateState()
	if err != nil {
		slog.Error("failed to generate state", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Encode CLI info in state: serverState|port|clientState
	combinedState := base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("%s|%s|%s", serverState, port, clientState)))

	// Set state cookie for validation
	h.sessions.SetCLIStateCookie(w, serverState)

	// Redirect to GitHub
	http.Redirect(w, r, h.github.GetAuthURL(combinedState), http.StatusFound)
}

// CLICallback handles the OAuth callback for CLI authentication.
// Creates an API token and redirects to the CLI's local callback server.
func (h *Handlers) CLICallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Decode combined state
	encodedState := r.URL.Query().Get("state")
	stateBytes, err := base64.URLEncoding.DecodeString(encodedState)
	if err != nil {
		slog.Warn("invalid state encoding")
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}

	parts := strings.SplitN(string(stateBytes), "|", 3)
	if len(parts) != 3 {
		slog.Warn("invalid state format")
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}

	serverState, port, clientState := parts[0], parts[1], parts[2]

	// Validate server state
	if !h.validateCLIState(r, serverState) {
		slog.Warn("invalid OAuth state")
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}
	h.sessions.ClearCLIStateCookie(w)

	// Check for error from GitHub
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		errDesc := r.URL.Query().Get("error_description")
		slog.Warn("OAuth error from GitHub", "error", errMsg, "description", errDesc)
		h.redirectCLIError(w, r, port, clientState, "Authorization denied: "+errDesc)
		return
	}

	// Exchange code for token
	code := r.URL.Query().Get("code")
	if code == "" {
		h.redirectCLIError(w, r, port, clientState, "Missing authorization code")
		return
	}

	token, err := h.github.ExchangeCode(ctx, code)
	if err != nil {
		slog.Error("failed to exchange code", "error", err)
		h.redirectCLIError(w, r, port, clientState, "Failed to authenticate")
		return
	}

	// Get user info
	user, err := h.github.GetUser(ctx, token.AccessToken)
	if err != nil {
		slog.Error("failed to get user", "error", err)
		h.redirectCLIError(w, r, port, clientState, "Failed to get user info")
		return
	}

	// Check authorization
	if !h.authorizer.IsAuthorized(ctx, user.Login, token.AccessToken) {
		slog.Warn("user not authorized", "username", user.Login)
		h.redirectCLIError(w, r, port, clientState, "You are not authorized to access this application")
		return
	}

	// Generate hostname for token name
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	tokenName := fmt.Sprintf("CLI - %s", hostname)

	// Create API token
	apiToken, _, err := h.tokens.GenerateToken(ctx, fmt.Sprintf("%d", user.ID), user.Login, tokenName)
	if err != nil {
		slog.Error("failed to create API token", "error", err)
		h.redirectCLIError(w, r, port, clientState, "Failed to create API token")
		return
	}

	slog.Info("CLI login successful", "username", user.Login, "user_id", user.ID)

	// Redirect to CLI callback with token
	callbackURL := fmt.Sprintf("http://localhost:%s/callback?token=%s&state=%s&user_id=%d&username=%s",
		port,
		url.QueryEscape(apiToken),
		url.QueryEscape(clientState),
		user.ID,
		url.QueryEscape(user.Login),
	)
	http.Redirect(w, r, callbackURL, http.StatusFound)
}

// RevokeToken revokes an API token. Requires authentication via session or token.
func (h *Handlers) RevokeToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get token ID from request
	tokenID := r.URL.Query().Get("token_id")
	if tokenID == "" {
		http.Error(w, "Missing token_id parameter", http.StatusBadRequest)
		return
	}

	// Get current user from session
	session, err := h.sessions.GetSessionFromRequest(r)
	if err != nil || session == nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Get all user's tokens to verify ownership
	tokens, err := h.tokens.GetUserTokens(ctx, session.UserID)
	if err != nil {
		slog.Error("failed to get user tokens", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Check if token belongs to user
	found := false
	for _, t := range tokens {
		if t.ID == tokenID {
			found = true
			break
		}
	}

	if !found {
		http.Error(w, "Token not found", http.StatusNotFound)
		return
	}

	// Revoke the token
	if err := h.tokens.RevokeToken(ctx, tokenID); err != nil {
		slog.Error("failed to revoke token", "error", err)
		http.Error(w, "Failed to revoke token", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// validateCLIState validates the CLI OAuth state cookie.
func (h *Handlers) validateCLIState(r *http.Request, state string) bool {
	cookie, err := r.Cookie(CLIStateCookieName)
	if err != nil {
		return false
	}
	return cookie.Value == state
}

// redirectCLIError redirects to CLI callback with an error.
func (h *Handlers) redirectCLIError(w http.ResponseWriter, r *http.Request, port, state, errMsg string) {
	callbackURL := fmt.Sprintf("http://localhost:%s/callback?error=%s&state=%s",
		port,
		url.QueryEscape(errMsg),
		url.QueryEscape(state),
	)
	http.Redirect(w, r, callbackURL, http.StatusFound)
}
