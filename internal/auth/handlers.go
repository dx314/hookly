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
// Supports optional return_to parameter to redirect after login.
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	state, err := GenerateState()
	if err != nil {
		slog.Error("failed to generate state", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Store return_to in state if provided (format: state|return_to)
	returnTo := r.URL.Query().Get("return_to")
	if returnTo != "" {
		state = state + "|" + base64.URLEncoding.EncodeToString([]byte(returnTo))
	}

	h.sessions.SetStateCookie(w, state)
	http.Redirect(w, r, h.github.GetAuthURL(state), http.StatusFound)
}

// Callback handles the OAuth callback from GitHub.
func (h *Handlers) Callback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Validate state (may contain return_to: state|base64(return_to))
	fullState := r.URL.Query().Get("state")
	var returnTo string

	if parts := strings.SplitN(fullState, "|", 2); len(parts) == 2 {
		if decoded, err := base64.URLEncoding.DecodeString(parts[1]); err == nil {
			returnTo = string(decoded)
		}
	}

	if !ValidateState(r, fullState) {
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

	// Redirect to return_to or home
	redirectURL := "/"
	if returnTo != "" && strings.HasPrefix(returnTo, "/") {
		redirectURL = returnTo
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
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

// CLIRegister redirects to the Svelte page for CLI authorization.
// Requires the user to be logged in. If not, redirects to login first.
// GET /auth/cli/register?port=12345&state=xxx
func (h *Handlers) CLIRegister(w http.ResponseWriter, r *http.Request) {
	port := r.URL.Query().Get("port")
	state := r.URL.Query().Get("state")

	if port == "" || state == "" {
		http.Error(w, "Missing port or state parameter", http.StatusBadRequest)
		return
	}

	// Check if user is logged in
	session, _ := h.sessions.GetSessionFromRequest(r)
	if session == nil {
		// Redirect to login, then back here
		returnURL := fmt.Sprintf("/auth/cli/register?port=%s&state=%s", url.QueryEscape(port), url.QueryEscape(state))
		http.Redirect(w, r, "/auth/login?return_to="+url.QueryEscape(returnURL), http.StatusFound)
		return
	}

	// Redirect to Svelte page with session info
	svelteURL := fmt.Sprintf("/cli/register?port=%s&state=%s&username=%s",
		url.QueryEscape(port),
		url.QueryEscape(state),
		url.QueryEscape(session.Username),
	)
	http.Redirect(w, r, svelteURL, http.StatusFound)
}

// CLIAuthorize creates an API token and redirects to the CLI's local server.
// POST /cli/register/authorize
func (h *Handlers) CLIAuthorize(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if user is logged in
	session, _ := h.sessions.GetSessionFromRequest(r)
	if session == nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	port := r.FormValue("port")
	state := r.FormValue("state")

	if port == "" || state == "" {
		http.Error(w, "Missing port or state", http.StatusBadRequest)
		return
	}

	// Generate hostname for token name
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	tokenName := fmt.Sprintf("CLI - %s", hostname)

	// Create API token
	apiToken, _, err := h.tokens.GenerateToken(ctx, session.UserID, session.Username, tokenName)
	if err != nil {
		slog.Error("failed to create API token", "error", err)
		http.Error(w, "Failed to create API token", http.StatusInternalServerError)
		return
	}

	slog.Info("CLI authorized", "username", session.Username, "user_id", session.UserID)

	// Redirect to CLI callback with token
	callbackURL := fmt.Sprintf("http://localhost:%s/callback?token=%s&state=%s&user_id=%s&username=%s",
		port,
		url.QueryEscape(apiToken),
		url.QueryEscape(state),
		url.QueryEscape(session.UserID),
		url.QueryEscape(session.Username),
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

