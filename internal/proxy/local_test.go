package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	hooklyv1 "hooks.dx314.com/internal/api/hookly/v1"
)

// upstreamRecorder is the local service behind a proxy.
type upstreamRecorder struct {
	hits   atomic.Int32
	last   atomic.Pointer[http.Request]
	body   atomic.Pointer[string]
	delay  time.Duration
	bigLen int
}

func (u *upstreamRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u.hits.Add(1)
	body, _ := io.ReadAll(r.Body)
	s := string(body)
	u.body.Store(&s)
	u.last.Store(r)

	if u.delay > 0 {
		time.Sleep(u.delay)
	}
	switch r.URL.Path {
	case "/base/app/redirect":
		http.Redirect(w, r, "/app/", http.StatusFound)
	case "/base/app/big":
		w.Write(make([]byte, u.bigLen))
	default:
		w.Header().Set("Content-Type", "application/json")
		w.Header().Add("Set-Cookie", "a=1")
		w.Header().Add("Set-Cookie", "b=2")
		w.Header().Set("Connection", "close")
		w.Header().Set("X-Upstream", "yes")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"ok":true}`))
	}
}

func newTestForwarder(t *testing.T, up *upstreamRecorder) (*Forwarder, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(up)
	t.Cleanup(srv.Close)
	f, err := NewForwarder([]Upstream{{Name: "homeboy", URL: srv.URL + "/base", Paths: []string{"/app/"}}})
	if err != nil {
		t.Fatalf("NewForwarder: %v", err)
	}
	return f, srv
}

func TestForwarderForwardsAllowedPath(t *testing.T) {
	up := &upstreamRecorder{}
	f, _ := newTestForwarder(t, up)

	resp := f.Handle(context.Background(), &hooklyv1.HttpRequest{
		RequestId: "r1",
		Proxy:     "homeboy",
		Method:    http.MethodPost,
		Path:      "/app/items/a%20b",
		RawQuery:  "x=1&y=2",
		Headers: []*hooklyv1.HttpHeader{
			{Name: "Content-Type", Value: "application/json"},
			{Name: "Authorization", Value: "tma abc"},
			{Name: "X-Forwarded-Host", Value: "hooks.dx314.com"},
			{Name: "Host", Value: "evil.example"},
			{Name: "Connection", Value: "close"},
			{Name: "Upgrade", Value: "websocket"},
			{Name: "Transfer-Encoding", Value: "chunked"},
		},
		Body: []byte(`{"n":1}`),
	})

	if resp.RequestId != "r1" || resp.Error != "" || resp.Status != http.StatusCreated {
		t.Fatalf("response = %+v", resp)
	}
	if string(resp.Body) != `{"ok":true}` {
		t.Errorf("body = %q", resp.Body)
	}
	got := http.Header{}
	DecodeHeaders(got, resp.Headers)
	if got.Get("X-Upstream") != "yes" || got.Get("Content-Type") != "application/json" {
		t.Errorf("response headers = %v", got)
	}
	if c := got.Values("Set-Cookie"); len(c) != 2 {
		t.Errorf("Set-Cookie = %v, want 2 values", c)
	}
	if got.Get("Connection") != "" {
		t.Error("hop-by-hop Connection header came back")
	}

	r := up.last.Load()
	if r == nil {
		t.Fatal("upstream not called")
	}
	if r.Method != http.MethodPost || r.URL.Path != "/base/app/items/a b" || r.URL.RawQuery != "x=1&y=2" {
		t.Errorf("upstream saw %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
	}
	if *up.body.Load() != `{"n":1}` {
		t.Errorf("upstream body = %q", *up.body.Load())
	}
	if r.Header.Get("Authorization") != "tma abc" || r.Header.Get("X-Forwarded-Host") != "hooks.dx314.com" {
		t.Errorf("upstream headers = %v", r.Header)
	}
	if strings.Contains(r.Host, "evil") {
		t.Errorf("request chose the Host: %q", r.Host)
	}
	for _, name := range []string{"Connection", "Upgrade", "Transfer-Encoding"} {
		if r.Header.Get(name) != "" {
			t.Errorf("hop-by-hop %s reached upstream", name)
		}
	}
}

func TestForwarderRejectsOutsideAllowList(t *testing.T) {
	up := &upstreamRecorder{}
	f, _ := newTestForwarder(t, up)

	tests := []struct {
		name   string
		proxy  string
		path   string
		method string
		status int32
	}{
		{"other prefix", "homeboy", "/admin", http.MethodGet, http.StatusNotFound},
		{"prefix lookalike", "homeboy", "/application", http.MethodGet, http.StatusNotFound},
		{"root", "homeboy", "/", http.MethodGet, http.StatusNotFound},
		{"traversal", "homeboy", "/app/../admin", http.MethodGet, http.StatusBadRequest},
		{"encoded traversal", "homeboy", "/app/%2e%2e/admin", http.MethodGet, http.StatusBadRequest},
		{"encoded slash traversal", "homeboy", "/app/..%2fadmin", http.MethodGet, http.StatusBadRequest},
		{"backslash", "homeboy", "/app/..%5cadmin", http.MethodGet, http.StatusBadRequest},
		{"relative", "homeboy", "app/x", http.MethodGet, http.StatusBadRequest},
		{"unknown proxy", "other", "/app/", http.MethodGet, http.StatusNotFound},
		{"bad method", "homeboy", "/app/", "GET X", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := f.Handle(context.Background(), &hooklyv1.HttpRequest{Proxy: tt.proxy, Method: tt.method, Path: tt.path})
			if resp.Error != "" || resp.Status != tt.status {
				t.Errorf("status = %d error = %q, want %d", resp.Status, resp.Error, tt.status)
			}
		})
	}
	if up.hits.Load() != 0 {
		t.Errorf("upstream was called %d times", up.hits.Load())
	}

	// The prefix itself, with or without the slash, is inside
	for _, p := range []string{"/app", "/app/"} {
		if resp := f.Handle(context.Background(), &hooklyv1.HttpRequest{Proxy: "homeboy", Method: "GET", Path: p}); resp.Status != http.StatusCreated {
			t.Errorf("%s: status %d error %q", p, resp.Status, resp.Error)
		}
	}
}

func TestForwarderReturnsRedirectAsIs(t *testing.T) {
	f, _ := newTestForwarder(t, &upstreamRecorder{})
	resp := f.Handle(context.Background(), &hooklyv1.HttpRequest{Proxy: "homeboy", Method: "GET", Path: "/app/redirect"})
	if resp.Status != http.StatusFound {
		t.Fatalf("status = %d error = %q, want 302", resp.Status, resp.Error)
	}
	got := http.Header{}
	DecodeHeaders(got, resp.Headers)
	if got.Get("Location") != "/app/" {
		t.Errorf("Location = %q", got.Get("Location"))
	}
}

func TestForwarderTimeoutAndLimits(t *testing.T) {
	up := &upstreamRecorder{delay: 300 * time.Millisecond, bigLen: 2048}
	f, _ := newTestForwarder(t, up)
	f.SetTimeout(50 * time.Millisecond)

	resp := f.Handle(context.Background(), &hooklyv1.HttpRequest{Proxy: "homeboy", Method: "GET", Path: "/app/"})
	if resp.Error == "" || !strings.HasPrefix(resp.Error, "upstream:") {
		t.Errorf("timeout: error = %q, want upstream error", resp.Error)
	}

	up.delay = 0
	f.SetTimeout(5 * time.Second)
	f.SetMaxBody(1024)
	resp = f.Handle(context.Background(), &hooklyv1.HttpRequest{Proxy: "homeboy", Method: "GET", Path: "/app/big"})
	if !strings.Contains(resp.Error, "larger than") {
		t.Errorf("big response: error = %q status = %d", resp.Error, resp.Status)
	}
	resp = f.Handle(context.Background(), &hooklyv1.HttpRequest{Proxy: "homeboy", Method: "POST", Path: "/app/", Body: make([]byte, 1025)})
	if resp.Status != http.StatusRequestEntityTooLarge {
		t.Errorf("big request: status = %d error = %q", resp.Status, resp.Error)
	}

	unreachable, err := NewForwarder([]Upstream{{Name: "down", URL: "http://127.0.0.1:1", Paths: []string{"/"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp := unreachable.Handle(context.Background(), &hooklyv1.HttpRequest{Proxy: "down", Method: "GET", Path: "/x"}); resp.Error == "" {
		t.Errorf("unreachable upstream: got status %d, want error", resp.Status)
	}
}

func TestValidateUpstream(t *testing.T) {
	bad := []Upstream{
		{Name: "", URL: "http://127.0.0.1:1", Paths: []string{"/"}},
		{Name: "has space", URL: "http://127.0.0.1:1", Paths: []string{"/"}},
		{Name: "ok", URL: "", Paths: []string{"/"}},
		{Name: "ok", URL: "127.0.0.1:8790", Paths: []string{"/"}},
		{Name: "ok", URL: "ftp://127.0.0.1", Paths: []string{"/"}},
		{Name: "ok", URL: "http://127.0.0.1:8790?x=1", Paths: []string{"/"}},
		{Name: "ok", URL: "http://127.0.0.1:8790"},
		{Name: "ok", URL: "http://127.0.0.1:8790", Paths: []string{"app/"}},
		{Name: "ok", URL: "http://127.0.0.1:8790", Paths: []string{"/app/../"}},
	}
	for _, u := range bad {
		if err := ValidateUpstream(u); err == nil {
			t.Errorf("%+v: want error", u)
		}
	}
	if err := ValidateUpstream(Upstream{Name: "homeboy", URL: "http://127.0.0.1:8790", Paths: []string{"/app/"}}); err != nil {
		t.Errorf("valid upstream rejected: %v", err)
	}
	if _, err := NewForwarder([]Upstream{
		{Name: "a", URL: "http://127.0.0.1:1", Paths: []string{"/"}},
		{Name: "a", URL: "http://127.0.0.1:2", Paths: []string{"/"}},
	}); err == nil {
		t.Error("duplicate name accepted")
	}
	f, _ := NewForwarder(nil)
	if f.Names() != nil {
		t.Errorf("Names() = %v, want nil", f.Names())
	}
}
