package cli

import (
	"net/http"

	"connectrpc.com/connect"

	"hooks.dx314.com/internal/api/hookly/v1/hooklyv1connect"
)

// Client provides an authenticated ConnectRPC client for the CLI.
type Client struct {
	Edge hooklyv1connect.EdgeServiceClient
}

// NewClient creates a new authenticated ConnectRPC client.
func NewClient(edgeURL, token string) *Client {
	httpClient := &http.Client{
		Transport: &bearerAuthTransport{
			token: token,
			base:  http.DefaultTransport,
		},
	}

	edgeClient := hooklyv1connect.NewEdgeServiceClient(
		httpClient,
		edgeURL,
		connect.WithGRPC(),
	)

	return &Client{
		Edge: edgeClient,
	}
}

// bearerAuthTransport adds Bearer token authentication to HTTP requests.
type bearerAuthTransport struct {
	token string
	base  http.RoundTripper
}

func (t *bearerAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request to avoid mutating the original
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(clone)
}
