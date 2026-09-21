package proxy

import (
	"net/http"
	"testing"

	hooklyv1 "hooks.dx314.com/internal/api/hookly/v1"
)

type hooklyHeader = hooklyv1.HttpHeader

func TestCleanPath(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"", "/", true},
		{"/", "/", true},
		{"/app/", "/app/", true},
		{"/app/index.html", "/app/index.html", true},
		{"/app/a%20b", "/app/a b", true},
		{"app", "", false},
		{"/app/../secret", "", false},
		{"/app/%2e%2e/secret", "", false},
		{"/app/%2E%2E", "", false},
		{"/app/..", "", false},
		{"/../", "", false},
		{"/app/./x", "", false},
		{"/app//x", "", false},
		{"/app/%2f..%2fx", "", false},
		{"/app/x%5c..", "", false},
		{"/app/x%00", "", false},
		{"/app/%zz", "", false},
	}
	for _, tt := range tests {
		got, err := CleanPath(tt.in)
		if (err == nil) != tt.ok {
			t.Errorf("CleanPath(%q) err = %v, want ok=%v", tt.in, err, tt.ok)
			continue
		}
		if tt.ok && got != tt.want {
			t.Errorf("CleanPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAllowed(t *testing.T) {
	prefixes := []string{"/app/", "/api"}
	tests := map[string]bool{
		"/app":         true,
		"/app/":        true,
		"/app/x/y":     true,
		"/application": false,
		"/api":         true,
		"/api/":        true,
		"/api/v1":      true,
		"/apix":        false,
		"/":            false,
		"/other":       false,
	}
	for p, want := range tests {
		if got := Allowed(p, prefixes); got != want {
			t.Errorf("Allowed(%q) = %v, want %v", p, got, want)
		}
	}
	if !Allowed("/anything", []string{"/"}) {
		t.Error("prefix / must allow everything")
	}
}

func TestEncodeDecodeHeadersStripHopByHop(t *testing.T) {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Add("Set-Cookie", "a=1")
	h.Add("Set-Cookie", "b=2")
	h.Set("Connection", "keep-alive, X-Custom-Hop")
	h.Set("X-Custom-Hop", "drop me")
	h.Set("Keep-Alive", "timeout=5")
	h.Set("Transfer-Encoding", "chunked")
	h.Set("Upgrade", "websocket")
	h.Set("Proxy-Authorization", "secret")
	h.Set("Host", "evil.example")
	h.Set("Content-Length", "99")
	h.Set("Authorization", "Bearer x")

	wire := EncodeHeaders(h)
	got := http.Header{}
	DecodeHeaders(got, wire)

	for _, name := range []string{"Connection", "X-Custom-Hop", "Keep-Alive", "Transfer-Encoding", "Upgrade", "Proxy-Authorization", "Host", "Content-Length"} {
		if got.Get(name) != "" {
			t.Errorf("%s crossed the wire", name)
		}
	}
	if got.Get("Content-Type") != "application/json" || got.Get("Authorization") != "Bearer x" {
		t.Errorf("end-to-end headers lost: %v", got)
	}
	if cookies := got.Values("Set-Cookie"); len(cookies) != 2 || cookies[0] != "a=1" || cookies[1] != "b=2" {
		t.Errorf("Set-Cookie = %v, want both values in order", cookies)
	}

	// Decoding refuses hop-by-hop and malformed names even if a peer sends them
	smuggled := http.Header{}
	DecodeHeaders(smuggled, EncodeHeaders(http.Header{"X-Ok": {"1"}}))
	DecodeHeaders(smuggled, []*hooklyHeader{{Name: "Transfer-Encoding", Value: "chunked"}, {Name: "Bad Name", Value: "x"}, {Name: "", Value: "x"}})
	if len(smuggled) != 1 || smuggled.Get("X-Ok") != "1" {
		t.Errorf("DecodeHeaders let something through: %v", smuggled)
	}
}
