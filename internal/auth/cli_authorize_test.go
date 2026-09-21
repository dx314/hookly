package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"hooks.dx314.com/internal/db"
	"hooks.dx314.com/internal/logincode"
)

// CLIAuthorize must not depend on the browser reaching the CLI's localhost:
// it hands over a login code (in the URL fragment) that works from any machine.
func TestCLIAuthorizeIssuesLoginCode(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer conn.Close()
	queries := db.New(conn)

	sessions := NewSessionManager(queries, false, "/")
	tokens := NewTokenManager(queries)
	handlers := NewHandlers(nil, sessions, nil, tokens)

	session, err := sessions.CreateSession(ctx, &GitHubUser{ID: 42, Login: "alex"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	cookieRec := httptest.NewRecorder()
	sessions.SetSessionCookie(cookieRec, session)

	form := url.Values{"port": {"38805"}, "state": {"state-123"}}
	req := httptest.NewRequest(http.MethodPost, "/auth/cli/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookieRec.Result().Cookies() {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	handlers.CLIAuthorize(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 (body: %s)", rec.Code, rec.Body.String())
	}
	location, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse location: %v", err)
	}
	if location.Host != "" || location.Path != "/cli/login/code" {
		t.Errorf("redirects to %q, want the edge's own /cli/login/code page", location.String())
	}
	if location.RawQuery != "" {
		t.Errorf("token must travel in the fragment, not the query: %q", location.RawQuery)
	}

	fragment, _ := url.ParseQuery(location.Fragment)
	if fragment.Get("port") != "38805" {
		t.Errorf("port = %q", fragment.Get("port"))
	}
	payload, err := logincode.Decode(fragment.Get("code"))
	if err != nil {
		t.Fatalf("decode login code: %v", err)
	}
	if payload.State != "state-123" || payload.Username != "alex" || payload.UserID != session.UserID {
		t.Errorf("payload = %+v", payload)
	}

	// The code carries a working API token for that user
	token, err := tokens.ValidateToken(ctx, payload.Token)
	if err != nil {
		t.Fatalf("token from login code is not valid: %v", err)
	}
	if token.UserID != session.UserID {
		t.Errorf("token user = %q, want %q", token.UserID, session.UserID)
	}
}
