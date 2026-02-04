package relay

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"

	hooklyv1 "hooks.dx314.com/internal/api/hookly/v1"
	"hooks.dx314.com/internal/api/hookly/v1/hooklyv1connect"
	"hooks.dx314.com/internal/config"
	"hooks.dx314.com/internal/webhook"
)

const (
	initialBackoff  = 1 * time.Second
	maxBackoff      = 60 * time.Second
	clientHeartbeat = 30 * time.Second
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
func (c *Client) Run(ctx context.Context) error {
	backoff := initialBackoff

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		slog.Info("connecting to edge", "url", c.config.EdgeURL, "hub_id", c.config.HubID)

		err := c.connect(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			slog.Error("connection failed", "error", err, "retry_in", backoff)

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

func (c *Client) connect(ctx context.Context) error {
	// Create ConnectRPC client
	client := hooklyv1connect.NewRelayServiceClient(
		http.DefaultClient,
		c.config.EdgeURL,
	)

	// Open bidirectional stream
	stream := client.Stream(ctx)

	// Send authentication message with endpoint IDs
	timestamp := time.Now().Unix()
	signature := GenerateHMAC(c.config.HubID, timestamp, c.config.Secret)

	if err := stream.Send(&hooklyv1.StreamRequest{
		Message: &hooklyv1.StreamRequest_Connect{
			Connect: &hooklyv1.ConnectRequest{
				HubId:       c.config.HubID,
				Timestamp:   timestamp,
				Signature:   signature,
				EndpointIds: c.config.EndpointIDs(),
			},
		},
	}); err != nil {
		return err
	}

	// Wait for auth response
	resp, err := stream.Receive()
	if err != nil {
		return err
	}

	authResp := resp.GetConnectResponse()
	if authResp == nil {
		return errors.New("expected connect response")
	}
	if !authResp.Success {
		return errors.New("authentication failed: " + authResp.Error)
	}

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
			}
		}
	}()
	defer close(heartbeatDone)

	// Process messages
	for {
		msg, err := stream.Receive()
		if err != nil {
			if errors.Is(err, io.EOF) {
				slog.Info("connection closed by server")
				return nil
			}
			return err
		}

		switch m := msg.Message.(type) {
		case *hooklyv1.StreamResponse_Webhook:
			c.handleWebhook(ctx, stream, m.Webhook)
		case *hooklyv1.StreamResponse_Heartbeat:
			// Edge heartbeat received, just log it
			slog.Debug("heartbeat from edge", "timestamp", m.Heartbeat.Timestamp)
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
