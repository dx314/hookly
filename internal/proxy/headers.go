// Package proxy carries ordinary HTTP requests through the relay stream:
// the edge receives them at /p/{hub_id}/{name}/..., the hookly CLI forwards
// them to a local service named in hookly.yaml and returns the response.
//
// This file holds the rules shared by both ends: which headers cross the
// stream, and what a request path may look like.
package proxy

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	hooklyv1 "hooks.dx314.com/internal/api/hookly/v1"
)

const (
	// MaxBodyBytes caps a proxied request body and a proxied response body.
	MaxBodyBytes = 10 << 20

	// Capability is advertised by hubs that understand HttpRequest / HttpResponse.
	Capability = "proxy"
)

// NameRE is the shape of a proxy name (and of a hub ID in a /p/ URL).
var NameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// hopByHop are the headers that describe one connection, not the message
// (RFC 7230 §6.1), so they never cross the stream in either direction. Host
// and Content-Length are set afresh on each side.
var hopByHop = map[string]bool{
	"connection":          true,
	"keep-alive":          true,
	"proxy-authenticate":  true,
	"proxy-authorization": true,
	"proxy-connection":    true,
	"te":                  true,
	"trailer":             true,
	"transfer-encoding":   true,
	"upgrade":             true,
	"host":                true,
	"content-length":      true,
}

// connectionTokens lists the header names a Connection header marks hop-by-hop.
func connectionTokens(h http.Header) map[string]bool {
	tokens := map[string]bool{}
	for _, v := range h.Values("Connection") {
		for _, t := range strings.Split(v, ",") {
			if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
				tokens[t] = true
			}
		}
	}
	return tokens
}

// EncodeHeaders lists h for the wire, without hop-by-hop headers (fixed ones
// and those a Connection header names). Order is by name, values in order.
func EncodeHeaders(h http.Header) []*hooklyv1.HttpHeader {
	tokens := connectionTokens(h)
	names := make([]string, 0, len(h))
	for name := range h {
		names = append(names, name)
	}
	// Deterministic order keeps tests and logs stable
	sort.Strings(names)

	var out []*hooklyv1.HttpHeader
	for _, name := range names {
		lower := strings.ToLower(name)
		if hopByHop[lower] || tokens[lower] {
			continue
		}
		for _, v := range h[name] {
			out = append(out, &hooklyv1.HttpHeader{Name: name, Value: v})
		}
	}
	return out
}

// DecodeHeaders adds wire headers to dst, dropping hop-by-hop ones again so
// neither side can smuggle them through.
func DecodeHeaders(dst http.Header, hs []*hooklyv1.HttpHeader) {
	for _, h := range hs {
		if h == nil {
			continue
		}
		name := strings.TrimSpace(h.Name)
		if name == "" || hopByHop[strings.ToLower(name)] || !validHeaderName(name) {
			continue
		}
		dst.Add(name, h.Value)
	}
}

func validHeaderName(name string) bool {
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0:
		default:
			return false
		}
	}
	return true
}

// ErrBadPath is returned by CleanPath for a path that must not be forwarded.
var ErrBadPath = errors.New("invalid path")

// CleanPath validates a request path as it appears on the wire (percent
// encoded, without the /p/{hub_id}/{name} prefix) and returns it decoded.
// The path must be absolute, decode cleanly, and already be in canonical
// form: no "." or ".." segments (encoded or not), no backslashes, no empty
// segments other than a trailing slash, no NUL. Both ends apply it, so a
// request can never escape the prefixes a proxy allows.
func CleanPath(wire string) (string, error) {
	if wire == "" {
		return "/", nil
	}
	if !strings.HasPrefix(wire, "/") {
		return "", fmt.Errorf("%w: not absolute", ErrBadPath)
	}
	decoded, err := url.PathUnescape(wire)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrBadPath, err)
	}
	if strings.ContainsAny(decoded, "\\\x00") {
		return "", fmt.Errorf("%w: forbidden character", ErrBadPath)
	}
	for _, seg := range strings.Split(decoded, "/") {
		if seg == "." || seg == ".." {
			return "", fmt.Errorf("%w: dot segment", ErrBadPath)
		}
	}
	clean := path.Clean(decoded)
	if strings.HasSuffix(decoded, "/") && clean != "/" {
		clean += "/"
	}
	if clean != decoded {
		return "", fmt.Errorf("%w: not canonical", ErrBadPath)
	}
	return clean, nil
}

// Allowed reports whether a clean path falls under one of prefixes. A prefix
// matches itself with or without its trailing slash and everything below it,
// so "/app/" allows "/app", "/app/" and "/app/x" but not "/application".
func Allowed(clean string, prefixes []string) bool {
	for _, prefix := range prefixes {
		p := strings.TrimSuffix(prefix, "/")
		if clean == p || clean == p+"/" || strings.HasPrefix(clean, p+"/") {
			return true
		}
	}
	return false
}
