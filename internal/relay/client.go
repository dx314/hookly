package relay

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"

	hooklyv1 "hooks.dx314.com/internal/api/hookly/v1"
	"hooks.dx314.com/internal/api/hookly/v1/hooklyv1connect"
	"hooks.dx314.com/internal/config"
	"hooks.dx314.com/internal/webhook"
)

const (
	initialBackoff  = 1 * time.Second
	maxBackoff      = 60 * time.Second
	clientHeartbeat = 15 * time.Second
)

// Connection error types - permanent errors should not be retried
var (
	ErrTokenInvalid      = errors.New("token invalid or expired")
	ErrTokenRevoked      = errors.New("token revoked")
	ErrEndpointNotFound  = errors.New("endpoint not found")
	ErrEndpointForbidden = errors.New("endpoint access denied")
	ErrNoEndpoints       = errors.New("no endpoints configured")
)

// Client connects to the edge relay service and handles webhooks.
type Client struct {
	config    *config.HooklyConfig
	forwarder *webhook.Forwarder
}

// NewClient creates a new relay client from HooklyConfig.
func NewClient(cfg *config.HooklyConfig) *Client {
	return &Client{
		config:    cfg,
		forwarder: webhook.NewForwarder(),
	}
}

// Run connects to the edge and processes webhooks until context is cancelled.
// Automatically reconnects on disconnect with exponential backoff.
// Returns immediately on permanent errors (auth issues, endpoint not found).
func (c *Client) Run(ctx context.Context) error {
	backoff := initialBackoff

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		slog.Info("connecting to edge", "url", c.config.EdgeURL, "hub_id", c.config.GetHubID())

		err := c.connect(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}

			// Check for permanent errors that shouldn't be retried
			if isPermanentError(err) {
				slog.Error("connection failed (not retrying)", "error", err)
				return err
			}

			slog.Warn("connection failed, will retry", "error", err, "retry_in", backoff)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}

			// Increase backoff
			backoff = backoff * 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		} else {
			// Connection was clean, reset backoff
			backoff = initialBackoff
		}
	}
}

// isPermanentError returns true if the error should not be retried.
func isPermanentError(err error) bool {
	if errors.Is(err, ErrTokenInvalid) ||
		errors.Is(err, ErrTokenRevoked) ||
		errors.Is(err, ErrEndpointNotFound) ||
		errors.Is(err, ErrEndpointForbidden) ||
		errors.Is(err, ErrNoEndpoints) {
		return true
	}
	return false
}

func (c *Client) connect(ctx context.Context) error {
	// Create HTTP client with HTTP/2 keepalive to prevent proxy timeouts
	slog.Debug("creating HTTP/2 transport",
		"keepalive", "15s",
		"timeout", "30s",
		"read_idle_timeout", "15s",
		"ping_timeout", "5s",
	)
	httpClient := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: false,
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
				slog.Debug("TLS dial starting", "network", network, "addr", addr)
				dialer := &net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 15 * time.Second,
				}
				conn, err := tls.DialWithDialer(dialer, network, addr, cfg)
				if err != nil {
					slog.Debug("TLS dial failed", "error", err)
				} else {
					slog.Debug("TLS dial succeeded", "remote", conn.RemoteAddr())
				}
				return conn, err
			},
			ReadIdleTimeout: 15 * time.Second,
			PingTimeout:     5 * time.Second,
		},
	}

	// Create ConnectRPC client
	client := hooklyv1connect.NewRelayServiceClient(
		httpClient,
		c.config.EdgeURL,
	)

	// Open bidirectional stream
	stream := client.Stream(ctx)

	// Send authentication message with bearer token
	hubID := c.config.GetHubID()
	slog.Debug("sending auth message", "hub_id", hubID, "endpoints", len(c.config.EndpointIDs()))

	if err := stream.Send(&hooklyv1.StreamRequest{
		Message: &hooklyv1.StreamRequest_Connect{
			Connect: &hooklyv1.ConnectRequest{
				HubId:       hubID,
				Token:       c.config.Token,
				EndpointIds: c.config.EndpointIDs(),
			},
		},
	}); err != nil {
		return err
	}

	// Wait for auth response
	slog.Debug("waiting for auth response")
	resp, err := stream.Receive()
	if err != nil {
		return fmt.Errorf("connect to edge: %w", err)
	}

	authResp := resp.GetConnectResponse()
	if authResp == nil {
		return errors.New("unexpected response from server")
	}
	if !authResp.Success {
		slog.Debug("auth failed", "error", authResp.Error)
		return parseConnectError(authResp.Error)
	}

	slog.Debug("auth succeeded")
	slog.Info("connected to edge", "endpoints", c.config.EndpointIDs())

	// Start heartbeat sender
	heartbeatDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(clientHeartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatDone:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				slog.Debug("sending heartbeat")
				if err := stream.Send(&hooklyv1.StreamRequest{
					Message: &hooklyv1.StreamRequest_Heartbeat{
						Heartbeat: &hooklyv1.Heartbeat{
							Timestamp: time.Now().Unix(),
						},
					},
				}); err != nil {
					slog.Warn("failed to send heartbeat", "error", err)
					return
				}
				slog.Debug("heartbeat sent")
			}
		}
	}()
	defer close(heartbeatDone)

	// Process messages
	slog.Debug("entering message loop")
	for {
		msg, err := stream.Receive()
		if err != nil {
			if errors.Is(err, io.EOF) {
				slog.Info("connection closed by server")
				return nil
			}
			slog.Debug("stream receive error", "error", err)
			return err
		}

		switch m := msg.Message.(type) {
		case *hooklyv1.StreamResponse_Webhook:
			slog.Debug("received webhook message", "webhook_id", m.Webhook.Id)
			c.handleWebhook(ctx, stream, m.Webhook)
		case *hooklyv1.StreamResponse_Heartbeat:
			slog.Debug("heartbeat from edge", "timestamp", m.Heartbeat.Timestamp)
		default:
			slog.Debug("received unknown message type")
		}
	}
}

func (c *Client) handleWebhook(ctx context.Context, stream *connect.BidiStreamForClient[hooklyv1.StreamRequest, hooklyv1.StreamResponse], envelope *hooklyv1.WebhookEnvelope) {
	// Get destination URL, allowing local override
	destinationURL := c.config.GetDestination(envelope.EndpointId, envelope.DestinationUrl)

	slog.Info("received webhook",
		"webhook_id", envelope.Id,
		"endpoint_id", envelope.EndpointId,
		"destination", destinationURL,
		"attempt", envelope.Attempt,
	)

	// Forward webhook
	result := c.forwarder.Forward(
		ctx,
		destinationURL,
		envelope.Headers,
		envelope.Payload,
		envelope.Id,
		int(envelope.Attempt),
	)

	// Send ACK
	ack := &hooklyv1.DeliveryAck{
		WebhookId:        envelope.Id,
		Success:          result.Success,
		StatusCode:       int32(result.StatusCode),
		ErrorMessage:     result.Error,
		PermanentFailure: result.PermanentFailure,
	}

	if err := stream.Send(&hooklyv1.StreamRequest{
		Message: &hooklyv1.StreamRequest_Ack{
			Ack: ack,
		},
	}); err != nil {
		slog.Error("failed to send ACK", "webhook_id", envelope.Id, "error", err)
	}
}

// parseConnectError parses the server error string and returns a typed error.
// Server errors are in format "ERROR_CODE: human message"
func parseConnectError(serverError string) error {
	// Extract the error code (before the colon)
	code := serverError
	message := serverError
	if idx := strings.Index(serverError, ": "); idx > 0 {
		code = serverError[:idx]
		message = serverError[idx+2:]
	}

	// Map error codes to typed errors with helpful messages
	switch code {
	case "TOKEN_MISSING":
		return fmt.Errorf("%w: %s", ErrTokenInvalid, message)
	case "TOKEN_INVALID":
		return fmt.Errorf("%w: %s", ErrTokenInvalid, message)
	case "TOKEN_REVOKED":
		return fmt.Errorf("%w: %s", ErrTokenRevoked, message)
	case "NO_ENDPOINTS":
		return fmt.Errorf("%w: %s", ErrNoEndpoints, message)
	case "ENDPOINT_NOT_FOUND":
		return fmt.Errorf("%w: %s", ErrEndpointNotFound, message)
	case "ENDPOINT_ACCESS_DENIED":
		return fmt.Errorf("%w: %s", ErrEndpointForbidden, message)
	default:
		return fmt.Errorf("server error: %s", serverError)
	}
}
