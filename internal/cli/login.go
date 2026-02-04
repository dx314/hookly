package cli

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"time"
)

const (
	// CallbackTimeout is how long to wait for the OAuth callback.
	CallbackTimeout = 5 * time.Minute
)

// LoginResult contains the result of a successful login.
type LoginResult struct {
	Token    string
	UserID   string
	Username string
}

// Login performs the OAuth login flow.
// It starts a local server, opens the browser, and waits for the callback.
func Login(ctx context.Context, edgeURL string) (*LoginResult, error) {
	// Find an available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen on local port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	// Generate state for CSRF protection
	state, err := generateState()
	if err != nil {
		listener.Close()
		return nil, fmt.Errorf("generate state: %w", err)
	}

	// Create result channel
	resultCh := make(chan *LoginResult, 1)
	errCh := make(chan error, 1)

	// Create callback server
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		// Check for error
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			errCh <- errors.New(errMsg)
			// Redirect to error page on edge server
			errorURL := fmt.Sprintf("%s/cli/login/error?error=%s", edgeURL, url.QueryEscape(errMsg))
			http.Redirect(w, r, errorURL, http.StatusFound)
			return
		}

		// Validate state
		returnedState := r.URL.Query().Get("state")
		if returnedState != state {
			errCh <- errors.New("state mismatch")
			errorURL := fmt.Sprintf("%s/cli/login/error?error=%s", edgeURL, url.QueryEscape("state mismatch"))
			http.Redirect(w, r, errorURL, http.StatusFound)
			return
		}

		// Get token and user info
		token := r.URL.Query().Get("token")
		userID := r.URL.Query().Get("user_id")
		username := r.URL.Query().Get("username")

		if token == "" {
			errCh <- errors.New("missing token in callback")
			errorURL := fmt.Sprintf("%s/cli/login/error?error=%s", edgeURL, url.QueryEscape("missing token"))
			http.Redirect(w, r, errorURL, http.StatusFound)
			return
		}

		resultCh <- &LoginResult{
			Token:    token,
			UserID:   userID,
			Username: username,
		}

		// Redirect to success page on edge server
		successURL := fmt.Sprintf("%s/cli/login/success?username=%s", edgeURL, url.QueryEscape(username))
		http.Redirect(w, r, successURL, http.StatusFound)
	})

	server := &http.Server{Handler: mux}

	// Start server in goroutine
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			slog.Error("callback server error", "error", err)
		}
	}()

	// Build login URL
	loginURL := fmt.Sprintf("%s/auth/cli/register?port=%d&state=%s",
		edgeURL,
		port,
		url.QueryEscape(state),
	)

	// Open browser
	fmt.Printf("Opening browser for login...\n")
	fmt.Printf("If the browser doesn't open, visit: %s\n\n", loginURL)

	if err := openBrowser(loginURL); err != nil {
		slog.Warn("failed to open browser", "error", err)
	}

	// Wait for callback with timeout
	ctx, cancel := context.WithTimeout(ctx, CallbackTimeout)
	defer cancel()

	var result *LoginResult
	select {
	case result = <-resultCh:
		// Success
	case err := <-errCh:
		server.Close()
		return nil, err
	case <-ctx.Done():
		server.Close()
		return nil, fmt.Errorf("login timed out after %v", CallbackTimeout)
	}

	// Shutdown server
	server.Close()

	return result, nil
}

// generateState generates a random state string for CSRF protection.
func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// openBrowser opens the default browser with the given URL.
func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return exec.Command(cmd, args...).Start()
}
