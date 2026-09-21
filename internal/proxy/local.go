package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	hooklyv1 "hooks.dx314.com/internal/api/hookly/v1"
)

const (
	// UpstreamTimeout bounds one request to a local service. It is above the
	// edge's minimum wait so a long-poll (up to ~25s) answers normally.
	UpstreamTimeout = 40 * time.Second

	// MaxConcurrent is how many proxied requests the CLI forwards at once.
	// Long-polls hold a slot each, so it is generous; beyond it requests wait.
	MaxConcurrent = 64
)

// Upstream is one local service the CLI reverse-proxies: the `proxies:`
// entry of hookly.yaml.
type Upstream struct {
	Name  string
	URL   string   // http(s)://host:port[/base]; the path is joined to it
	Paths []string // Allowed path prefixes; anything else answers 404
}

// ValidateUpstream checks one `proxies:` entry.
func ValidateUpstream(u Upstream) error {
	if !NameRE.MatchString(u.Name) {
		return fmt.Errorf("name %q must be letters, digits, '.', '_' or '-' (up to 64)", u.Name)
	}
	if _, err := parseUpstreamURL(u.URL); err != nil {
		return fmt.Errorf("proxy %q: %w", u.Name, err)
	}
	if len(u.Paths) == 0 {
		return fmt.Errorf("proxy %q: paths is required (for example [\"/app/\"])", u.Name)
	}
	for _, p := range u.Paths {
		if !strings.HasPrefix(p, "/") {
			return fmt.Errorf("proxy %q: path %q must start with '/'", u.Name, p)
		}
		if _, err := CleanPath(p); err != nil {
			return fmt.Errorf("proxy %q: path %q: %w", u.Name, p, err)
		}
	}
	return nil
}

func parseUpstreamURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, errors.New("url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("url %q must start with http:// or https://", raw)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("url %q has no host", raw)
	}
	if u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return nil, fmt.Errorf("url %q must not have a query, fragment or user", raw)
	}
	return u, nil
}

// Forwarder answers HttpRequests on the CLI by calling the configured local
// service. It only ever forwards to a configured name and under its allowed
// prefixes; the request never chooses the host.
type Forwarder struct {
	upstreams map[string]upstream
	names     []string
	client    *http.Client
	maxBody   int64
}

type upstream struct {
	base  *url.URL
	paths []string
}

// NewForwarder builds a Forwarder from the `proxies:` entries. Each must pass
// ValidateUpstream.
func NewForwarder(upstreams []Upstream) (*Forwarder, error) {
	f := &Forwarder{
		upstreams: make(map[string]upstream, len(upstreams)),
		client: &http.Client{
			Timeout: UpstreamTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				// The 3xx goes back to the browser as-is
				return http.ErrUseLastResponse
			},
		},
		maxBody: MaxBodyBytes,
	}
	for _, u := range upstreams {
		if err := ValidateUpstream(u); err != nil {
			return nil, err
		}
		if _, dup := f.upstreams[u.Name]; dup {
			return nil, fmt.Errorf("proxy %q is listed twice", u.Name)
		}
		base, _ := parseUpstreamURL(u.URL)
		f.upstreams[u.Name] = upstream{base: base, paths: u.Paths}
		f.names = append(f.names, u.Name)
	}
	return f, nil
}

// SetTimeout replaces the upstream timeout (used by tests).
func (f *Forwarder) SetTimeout(d time.Duration) { f.client.Timeout = d }

// SetMaxBody replaces the response body cap (used by tests).
func (f *Forwarder) SetMaxBody(n int64) { f.maxBody = n }

// Names lists the configured proxy names, in configuration order, for
// ConnectRequest.proxies. Nil when nothing is configured.
func (f *Forwarder) Names() []string {
	if f == nil {
		return nil
	}
	return f.names
}

// Handle forwards one request and returns the response to send back. It
// never returns nil; a request that must not be forwarded gets a 404 or 400,
// an upstream that cannot be reached gets Error set (the edge answers 502).
func (f *Forwarder) Handle(ctx context.Context, req *hooklyv1.HttpRequest) *hooklyv1.HttpResponse {
	start := time.Now()
	resp := f.handle(ctx, req)
	resp.RequestId = req.RequestId

	attrs := []any{
		"proxy", req.Proxy,
		"method", req.Method,
		"path", req.Path,
		"status", resp.Status,
		"duration", time.Since(start).String(),
	}
	if resp.Error != "" {
		slog.Warn("proxy request failed", append(attrs, "error", resp.Error)...)
	} else {
		slog.Info("proxied request", attrs...)
	}
	return resp
}

func (f *Forwarder) handle(ctx context.Context, req *hooklyv1.HttpRequest) *hooklyv1.HttpResponse {
	up, ok := f.upstreams[req.Proxy]
	if !ok || f == nil {
		return textResponse(http.StatusNotFound, "no such proxy")
	}
	clean, err := CleanPath(req.Path)
	if err != nil {
		return textResponse(http.StatusBadRequest, "invalid path")
	}
	if !Allowed(clean, up.paths) {
		return textResponse(http.StatusNotFound, "not found")
	}
	if req.Method == "" || !validHeaderName(req.Method) { // method tokens share the header-name grammar
		return textResponse(http.StatusBadRequest, "invalid method")
	}
	if int64(len(req.Body)) > f.maxBody {
		return textResponse(http.StatusRequestEntityTooLarge, "request body too large")
	}

	// The host always comes from the config; only the path and query are the caller's
	target := *up.base
	target.Path = strings.TrimSuffix(up.base.Path, "/") + clean
	target.RawPath = ""
	target.RawQuery = req.RawQuery
	target.Fragment = ""

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, target.String(), bytes.NewReader(req.Body))
	if err != nil {
		return textResponse(http.StatusBadRequest, "invalid request")
	}
	httpReq.ContentLength = int64(len(req.Body))
	DecodeHeaders(httpReq.Header, req.Headers)

	httpResp, err := f.client.Do(httpReq)
	if err != nil {
		return &hooklyv1.HttpResponse{Error: "upstream: " + err.Error()}
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(httpResp.Body, f.maxBody+1))
	if err != nil {
		return &hooklyv1.HttpResponse{Error: "upstream body: " + err.Error()}
	}
	if int64(len(body)) > f.maxBody {
		return &hooklyv1.HttpResponse{Error: fmt.Sprintf("upstream response larger than %d bytes", f.maxBody)}
	}

	return &hooklyv1.HttpResponse{
		Status:  int32(httpResp.StatusCode),
		Headers: EncodeHeaders(httpResp.Header),
		Body:    body,
	}
}

func textResponse(status int, text string) *hooklyv1.HttpResponse {
	return &hooklyv1.HttpResponse{
		Status:  int32(status),
		Headers: []*hooklyv1.HttpHeader{{Name: "Content-Type", Value: "text/plain; charset=utf-8"}},
		Body:    []byte(text + "\n"),
	}
}
