package proxy

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	hooklyv1 "hooks.dx314.com/internal/api/hookly/v1"
	"hooks.dx314.com/internal/id"
)

// DefaultTimeout is how long the edge waits for a hub's HttpResponse unless
// PROXY_TIMEOUT says otherwise. Long-polls hold a request for ~25s, so this
// stays comfortably above that.
const DefaultTimeout = 45 * time.Second

// MinTimeout is the least PROXY_TIMEOUT may be set to.
const MinTimeout = 35 * time.Second

// Errors a Hub may return from Proxy.
var (
	// ErrDisconnected: the stream dropped before the response arrived.
	ErrDisconnected = errors.New("relay disconnected")
	// ErrBusy: the hub already has as many proxied requests in flight as allowed.
	ErrBusy = errors.New("too many requests in flight")
)

// Hub is a live relay connection that advertised the proxy capability and
// the requested name. Proxy sends req down the stream and waits for the
// matching HttpResponse or for ctx to end.
type Hub interface {
	Proxy(ctx context.Context, req *hooklyv1.HttpRequest) (*hooklyv1.HttpResponse, error)
}

// HubFinder looks up the connection that serves proxy name for hubID.
// It returns nil when no such hub is connected.
type HubFinder interface {
	FindProxy(hubID, name string) Hub
}

// Handler serves /p/{hub_id}/{name}/... on the edge: it turns the request
// into an HttpRequest, sends it to that hub and writes back its response.
// Nothing is verified or stored here; the local service does its own auth.
type Handler struct {
	hubs    HubFinder
	timeout time.Duration
	maxBody int64
	newID   func() string
}

// NewHandler creates the edge proxy handler. timeout <= 0 means DefaultTimeout.
func NewHandler(hubs HubFinder, timeout time.Duration) *Handler {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Handler{
		hubs:    hubs,
		timeout: timeout,
		maxBody: MaxBodyBytes,
		newID:   id.NewRequestID,
	}
}

// SetMaxBody replaces the request body cap (used by tests).
func (h *Handler) SetMaxBody(n int64) { h.maxBody = n }

// ServeHTTP handles one proxied request. Mount it at /p/*.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// /p/{hub}/{name}{rest}: split the escaped path so an encoded slash in a
	// segment cannot move the boundary.
	hubID, name, rest, ok := splitPrefix(r.URL.EscapedPath())
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	path, err := CleanPath(rest)
	if err != nil {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, h.maxBody))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	hub := h.hubs.FindProxy(hubID, name)
	if hub == nil {
		http.Error(w, "no relay connected for this service", http.StatusBadGateway)
		return
	}

	req := &hooklyv1.HttpRequest{
		RequestId: h.newID(),
		Proxy:     name,
		Method:    r.Method,
		Path:      rest, // still escaped; the CLI decodes once
		RawQuery:  r.URL.RawQuery,
		Headers:   forwardedHeaders(r, "/p/"+hubID+"/"+name),
		Body:      body,
	}
	if req.Path == "" {
		req.Path = "/"
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	resp, err := hub.Proxy(ctx, req)
	status := 0
	switch {
	case err == nil && resp.Error != "":
		status = http.StatusBadGateway
		http.Error(w, "bad gateway", status)
		slog.Warn("proxy upstream error", "hub_id", hubID, "proxy", name, "error", resp.Error)
	case errors.Is(err, context.DeadlineExceeded):
		status = http.StatusGatewayTimeout
		http.Error(w, "gateway timeout", status)
	case errors.Is(err, ErrBusy):
		status = http.StatusServiceUnavailable
		http.Error(w, "relay busy", status)
	case err != nil:
		if r.Context().Err() != nil {
			// Caller went away; nothing to write
			return
		}
		status = http.StatusBadGateway
		http.Error(w, "bad gateway", status)
	case resp.Status < 100 || resp.Status > 599:
		status = http.StatusBadGateway
		http.Error(w, "bad gateway", status)
	default:
		status = int(resp.Status)
		DecodeHeaders(w.Header(), resp.Headers)
		w.Header().Set("Content-Length", strconv.Itoa(len(resp.Body)))
		w.WriteHeader(status)
		if r.Method != http.MethodHead {
			_, _ = w.Write(resp.Body)
		}
	}

	slog.Info("proxied request",
		"hub_id", hubID,
		"proxy", name,
		"method", r.Method,
		"path", path,
		"status", status,
		"duration", time.Since(start).String(),
	)
}

// splitPrefix takes "/p/{hub}/{name}{rest}" apart. rest is "" or starts
// with "/". hub and name are unescaped and must look like names.
func splitPrefix(escaped string) (hubID, name, rest string, ok bool) {
	trimmed, found := strings.CutPrefix(escaped, "/p/")
	if !found {
		return "", "", "", false
	}
	hubSeg, after, found := strings.Cut(trimmed, "/")
	if !found {
		return "", "", "", false
	}
	nameSeg, rest, found := strings.Cut(after, "/")
	if found {
		rest = "/" + rest
	}
	hubID, err1 := url.PathUnescape(hubSeg)
	name, err2 := url.PathUnescape(nameSeg)
	if err1 != nil || err2 != nil || !NameRE.MatchString(hubID) || !NameRE.MatchString(name) {
		return "", "", "", false
	}
	return hubID, name, rest, true
}

// forwardedHeaders lists the request's headers for the wire, without
// hop-by-hop ones, plus the X-Forwarded-* the local service needs to know
// who called and how to build absolute URLs.
func forwardedHeaders(r *http.Request, prefix string) []*hooklyv1.HttpHeader {
	h := r.Header.Clone()

	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		if prior := h.Get("X-Forwarded-For"); prior != "" {
			h.Set("X-Forwarded-For", prior+", "+ip)
		} else {
			h.Set("X-Forwarded-For", ip)
		}
	} else if r.RemoteAddr != "" && h.Get("X-Forwarded-For") == "" {
		h.Set("X-Forwarded-For", r.RemoteAddr)
	}
	if h.Get("X-Forwarded-Proto") == "" {
		if r.TLS != nil {
			h.Set("X-Forwarded-Proto", "https")
		} else {
			h.Set("X-Forwarded-Proto", "http")
		}
	}
	if h.Get("X-Forwarded-Host") == "" && r.Host != "" {
		h.Set("X-Forwarded-Host", r.Host)
	}
	h.Set("X-Forwarded-Prefix", prefix)

	return EncodeHeaders(h)
}
