package webhook

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Forwarder forwards webhooks to destination URLs.
type Forwarder struct {
	client *http.Client
}

// ForwardResult contains the result of a webhook forward attempt.
type ForwardResult struct {
	StatusCode       int
	Success          bool
	PermanentFailure bool // True for 4xx errors
	Error            string
}

// NewForwarder creates a new webhook forwarder.
func NewForwarder() *Forwarder {
	return &Forwarder{
		client: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// Don't follow redirects - let the destination handle them
				return http.ErrUseLastResponse
			},
		},
	}
}

// Forward sends a webhook to the destination URL.
func (f *Forwarder) Forward(ctx context.Context, destinationURL string, headers map[string]string, payload []byte, webhookID string, attempt int) ForwardResult {
	result := ForwardResult{}

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, destinationURL, bytes.NewReader(payload))
	if err != nil {
		result.Error = fmt.Sprintf("create request: %v", err)
		return result
	}

	// Copy filtered headers
	for name, value := range headers {
		if shouldForwardHeader(name) {
			req.Header.Set(name, value)
		}
	}

	// Add Hookly-specific headers
	req.Header.Set("X-Hookly-Webhook-Id", webhookID)
	req.Header.Set("X-Hookly-Attempt", fmt.Sprintf("%d", attempt))

	// Ensure Content-Type is set
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	slog.Debug("forwarding webhook",
		"webhook_id", webhookID,
		"destination", destinationURL,
		"attempt", attempt,
		"payload_size", len(payload),
		"header_count", len(headers),
	)

	// Send request
	start := time.Now()
	resp, err := f.client.Do(req)
	if err != nil {
		result.Error = fmt.Sprintf("network error: %v", err)
		slog.Warn("forward failed",
			"webhook_id", webhookID,
			"destination", destinationURL,
			"error", err,
		)
		return result
	}
	defer resp.Body.Close()

	// Drain and close body
	_, _ = io.Copy(io.Discard, resp.Body)

	result.StatusCode = resp.StatusCode

	elapsed := time.Since(start)

	// Determine result
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Success = true
		slog.Info("webhook delivered",
			"webhook_id", webhookID,
			"status", resp.StatusCode,
		)
		slog.Debug("forward details",
			"webhook_id", webhookID,
			"destination", destinationURL,
			"duration", elapsed.String(),
		)
	} else if isPermanentClientError(resp.StatusCode) {
		// Permanent client error - don't retry
		result.PermanentFailure = true
		result.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		slog.Warn("webhook failed (permanent)",
			"webhook_id", webhookID,
			"status", resp.StatusCode,
		)
		slog.Debug("forward details",
			"webhook_id", webhookID,
			"destination", destinationURL,
			"duration", elapsed.String(),
		)
	} else if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		// Retryable client error (e.g., 404 - server might not be running yet)
		result.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		slog.Warn("webhook failed (will retry)",
			"webhook_id", webhookID,
			"status", resp.StatusCode,
		)
		slog.Debug("forward details",
			"webhook_id", webhookID,
			"destination", destinationURL,
			"duration", elapsed.String(),
		)
	} else {
		// Server error - transient failure, will retry
		result.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		slog.Warn("webhook failed (will retry)",
			"webhook_id", webhookID,
			"status", resp.StatusCode,
		)
		slog.Debug("forward details",
			"webhook_id", webhookID,
			"destination", destinationURL,
			"duration", elapsed.String(),
		)
	}

	return result
}

// isPermanentClientError returns true for 4xx errors that indicate a permanent
// problem that won't be fixed by retrying (bad payload, auth failure, etc.).
// Returns false for errors like 404 that may be transient (server not running).
func isPermanentClientError(statusCode int) bool {
	switch statusCode {
	case http.StatusBadRequest,           // 400 - malformed request
		http.StatusUnauthorized,          // 401 - auth required
		http.StatusForbidden,             // 403 - access denied
		http.StatusMethodNotAllowed,      // 405 - wrong HTTP method
		http.StatusGone,                  // 410 - permanently removed
		http.StatusUnsupportedMediaType,  // 415 - wrong content type
		http.StatusUnprocessableEntity,   // 422 - validation failed
		http.StatusTooManyRequests:       // 429 - rate limited (permanent in webhook context)
		return true
	default:
		return false
	}
}

// shouldForwardHeader returns true if the header should be forwarded.
func shouldForwardHeader(name string) bool {
	// Normalize header name
	name = strings.ToLower(name)

	// Headers to exclude
	excludeHeaders := map[string]bool{
		"host":              true,
		"content-length":    true,
		"connection":        true,
		"keep-alive":        true,
		"transfer-encoding": true,
		"te":                true,
		"trailer":           true,
		"upgrade":           true,
	}

	if excludeHeaders[name] {
		return false
	}

	// Always include Content-Type and X-* headers
	if name == "content-type" || strings.HasPrefix(name, "x-") {
		return true
	}

	// Include webhook-specific headers
	webhookHeaders := []string{
		"stripe-signature",
		"github-webhook",
		"authorization",
		"user-agent",
	}

	for _, h := range webhookHeaders {
		if name == h {
			return true
		}
	}

	// Include by default
	return true
}
