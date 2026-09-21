package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	hooklyv1 "hooks.dx314.com/internal/api/hookly/v1"
)

// fakeHub stands in for a HubConnection.
type fakeHub struct {
	got   *hooklyv1.HttpRequest
	resp  *hooklyv1.HttpResponse
	err   error
	block bool // wait for ctx, like a hub that never answers
}

func (f *fakeHub) Proxy(ctx context.Context, req *hooklyv1.HttpRequest) (*hooklyv1.HttpResponse, error) {
	f.got = req
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return f.resp, f.err
}

// fakeFinder maps "hub/name" to a hub.
type fakeFinder map[string]*fakeHub

func (f fakeFinder) FindProxy(hubID, name string) Hub {
	h, ok := f[hubID+"/"+name]
	if !ok {
		return nil
	}
	return h
}

func serve(t *testing.T, h *Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestEdgeHandlerForwardsAndWritesResponse(t *testing.T) {
	hub := &fakeHub{resp: &hooklyv1.HttpResponse{
		Status: http.StatusTeapot,
		Headers: []*hooklyv1.HttpHeader{
			{Name: "Content-Type", Value: "text/plain"},
			{Name: "Set-Cookie", Value: "a=1"},
			{Name: "Set-Cookie", Value: "b=2"},
			{Name: "Connection", Value: "close"},
			{Name: "Transfer-Encoding", Value: "chunked"},
		},
		Body: []byte("hello"),
	}}
	h := NewHandler(fakeFinder{"infocube/homeboy": hub}, 0)
	h.newID = func() string { return "req-1" }

	req := httptest.NewRequest(http.MethodPost, "https://hooks.dx314.com/p/infocube/homeboy/app/items/a%20b?x=1&y=2", strings.NewReader(`{"n":1}`))
	req.RemoteAddr = "203.0.113.9:4444"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "tma abc")
	req.Header.Set("Cookie", "s=1")
	req.Header.Set("Connection", "keep-alive, X-Hop")
	req.Header.Set("X-Hop", "drop")
	req.Header.Set("Upgrade", "websocket")
	rec := serve(t, h, req)

	if rec.Code != http.StatusTeapot || rec.Body.String() != "hello" {
		t.Fatalf("edge answered %d %q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Length") != "5" || rec.Header().Get("Content-Type") != "text/plain" {
		t.Errorf("response headers = %v", rec.Header())
	}
	if c := rec.Header().Values("Set-Cookie"); len(c) != 2 {
		t.Errorf("Set-Cookie = %v", c)
	}
	if rec.Header().Get("Connection") != "" || rec.Header().Get("Transfer-Encoding") != "" {
		t.Error("hop-by-hop response header reached the client")
	}

	got := hub.got
	if got == nil {
		t.Fatal("hub not called")
	}
	if got.RequestId != "req-1" || got.Proxy != "homeboy" || got.Method != http.MethodPost {
		t.Errorf("request = %+v", got)
	}
	if got.Path != "/app/items/a%20b" || got.RawQuery != "x=1&y=2" {
		t.Errorf("path = %q query = %q, want prefix stripped and encoding kept", got.Path, got.RawQuery)
	}
	if string(got.Body) != `{"n":1}` {
		t.Errorf("body = %q", got.Body)
	}
	hdr := http.Header{}
	DecodeHeaders(hdr, got.Headers)
	want := map[string]string{
		"Content-Type":       "application/json",
		"Authorization":      "tma abc",
		"Cookie":             "s=1",
		"X-Forwarded-For":    "203.0.113.9",
		"X-Forwarded-Proto":  "https",
		"X-Forwarded-Host":   "hooks.dx314.com",
		"X-Forwarded-Prefix": "/p/infocube/homeboy",
	}
	for k, v := range want {
		if hdr.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, hdr.Get(k), v)
		}
	}
	for _, k := range []string{"Connection", "X-Hop", "Upgrade", "Host"} {
		if hdr.Get(k) != "" {
			t.Errorf("%s was forwarded", k)
		}
	}
}

func TestEdgeHandlerPathVariants(t *testing.T) {
	hub := &fakeHub{resp: &hooklyv1.HttpResponse{Status: 200}}
	h := NewHandler(fakeFinder{"infocube/homeboy": hub}, 0)

	for url, wantPath := range map[string]string{
		"/p/infocube/homeboy":          "/",
		"/p/infocube/homeboy/":         "/",
		"/p/infocube/homeboy/app/":     "/app/",
		"/p/infocube/homeboy/app/x.js": "/app/x.js",
	} {
		hub.got = nil
		rec := serve(t, h, httptest.NewRequest(http.MethodGet, url, nil))
		if rec.Code != 200 || hub.got == nil || hub.got.Path != wantPath {
			t.Errorf("%s: code %d path %q, want %q", url, rec.Code, hub.got.GetPath(), wantPath)
		}
	}

	// X-Forwarded-* set by a proxy in front (Caddy) is kept and appended to
	hub.got = nil
	req := httptest.NewRequest(http.MethodGet, "http://edge/p/infocube/homeboy/app/", nil)
	req.RemoteAddr = "10.0.0.2:1"
	req.Header.Set("X-Forwarded-For", "198.51.100.7")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "hooks.dx314.com")
	serve(t, h, req)
	hdr := http.Header{}
	DecodeHeaders(hdr, hub.got.Headers)
	if hdr.Get("X-Forwarded-For") != "198.51.100.7, 10.0.0.2" || hdr.Get("X-Forwarded-Proto") != "https" || hdr.Get("X-Forwarded-Host") != "hooks.dx314.com" {
		t.Errorf("forwarded headers = %v", hdr)
	}
}

func TestEdgeHandlerErrors(t *testing.T) {
	tests := []struct {
		name string
		hubs fakeFinder
		url  string
		body int
		want int
	}{
		{"no hub", fakeFinder{}, "/p/infocube/homeboy/app/", 0, http.StatusBadGateway},
		{"other name", fakeFinder{"infocube/homeboy": {resp: &hooklyv1.HttpResponse{Status: 200}}}, "/p/infocube/schoolboy/app/", 0, http.StatusBadGateway},
		{"timeout", fakeFinder{"infocube/homeboy": {block: true}}, "/p/infocube/homeboy/app/", 0, http.StatusGatewayTimeout},
		{"busy", fakeFinder{"infocube/homeboy": {err: ErrBusy}}, "/p/infocube/homeboy/app/", 0, http.StatusServiceUnavailable},
		{"disconnected", fakeFinder{"infocube/homeboy": {err: ErrDisconnected}}, "/p/infocube/homeboy/app/", 0, http.StatusBadGateway},
		{"upstream error", fakeFinder{"infocube/homeboy": {resp: &hooklyv1.HttpResponse{Error: "upstream: refused"}}}, "/p/infocube/homeboy/app/", 0, http.StatusBadGateway},
		{"bad status", fakeFinder{"infocube/homeboy": {resp: &hooklyv1.HttpResponse{Status: 0}}}, "/p/infocube/homeboy/app/", 0, http.StatusBadGateway},
		{"traversal", fakeFinder{"infocube/homeboy": {block: true}}, "/p/infocube/homeboy/app/%2e%2e/secret", 0, http.StatusBadRequest},
		{"bad name", fakeFinder{"infocube/homeboy": {block: true}}, "/p/infocube/ho%20me/app/", 0, http.StatusNotFound},
		{"no name", fakeFinder{}, "/p/infocube", 0, http.StatusNotFound},
		{"too large", fakeFinder{"infocube/homeboy": {resp: &hooklyv1.HttpResponse{Status: 200}}}, "/p/infocube/homeboy/app/", 2000, http.StatusRequestEntityTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(tt.hubs, 50*time.Millisecond)
			h.SetMaxBody(1024)
			var body io.Reader
			if tt.body > 0 {
				body = strings.NewReader(strings.Repeat("x", tt.body))
			}
			rec := serve(t, h, httptest.NewRequest(http.MethodPost, tt.url, body))
			if rec.Code != tt.want {
				t.Errorf("code = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestEdgeHandlerCancelledCaller(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	hub := &fakeHub{err: errors.New("stream error")}
	h := NewHandler(fakeFinder{"infocube/homeboy": hub}, 0)
	rec := serve(t, h, httptest.NewRequest(http.MethodGet, "/p/infocube/homeboy/app/", nil).WithContext(ctx))
	if rec.Body.Len() != 0 {
		t.Errorf("wrote %q to a caller that went away", rec.Body.String())
	}
}
