package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"hooks.dx314.com/internal/proxy"
)

// HooklyConfig holds configuration for the hookly CLI.
type HooklyConfig struct {
	EdgeURL   string           `yaml:"edge_url"`
	HubID     string           `yaml:"hub_id,omitempty"` // Optional, auto-generated from hostname if empty
	Endpoints []EndpointConfig `yaml:"endpoints"`
	// Proxies are local services reachable from the edge at
	// /p/{hub_id}/{name}/... (reverse proxy through the stream). Optional:
	// without it nothing is ever proxied.
	Proxies []ProxyConfig `yaml:"proxies,omitempty"`
	// Token is loaded from credentials, not from YAML
	Token string `yaml:"-"`
}

// ProxyConfig is one local service the relay reverse-proxies.
type ProxyConfig struct {
	Name string `yaml:"name"` // Second segment of the public URL, /p/{hub_id}/{name}/
	URL  string `yaml:"url"`  // Local base URL, e.g. http://127.0.0.1:8790
	// Only requests under these path prefixes are forwarded (e.g. ["/app/"]);
	// anything else answers 404. Paths reach the service unchanged.
	Paths []string `yaml:"paths"`
}

// EndpointConfig declares an endpoint this hub handles. hookly creates it on
// the edge when it doesn't exist and keeps the edge in step with it (see
// internal/provision); fields left out are left as they are on the edge.
type EndpointConfig struct {
	// ID of the endpoint on the edge. Filled in by hookly when it creates or
	// finds the endpoint by name, so the file keeps pointing at the same one.
	ID string `yaml:"id,omitempty"`
	// Name on the edge. Identifies the endpoint when there is no id yet.
	Name string `yaml:"name,omitempty"`
	// URL is the public webhook URL to give the provider. Filled in by hookly;
	// never read.
	URL string `yaml:"url,omitempty"`
	// Provider decides how incoming webhooks are verified: stripe, github,
	// telegram, generic (HMAC-SHA256) or custom (see Verification).
	Provider string `yaml:"provider,omitempty"`
	// Secret is the signature secret (Telegram: the setWebhook secret_token).
	// SecretEnv names an environment variable holding it instead, so the
	// secret needn't be written in the file.
	Secret    string `yaml:"secret,omitempty"`
	SecretEnv string `yaml:"secret_env,omitempty"`
	// Verification configures the custom provider.
	Verification *VerificationConfig `yaml:"verification,omitempty"`
	Muted        *bool               `yaml:"muted,omitempty"`
	// Prune removes destinations on the edge that aren't listed here. Without
	// it they are left alone (and logged).
	Prune bool `yaml:"prune,omitempty"`
	// Legacy: URL of the endpoint's primary (first) destination.
	Destination string `yaml:"destination,omitempty"`
	// Destinations the endpoint fans out to, by name. The first one is the
	// primary when hookly creates the endpoint.
	Destinations DestinationList `yaml:"destinations,omitempty"`
}

// VerificationConfig is the custom provider's verification scheme.
type VerificationConfig struct {
	// Method: static, hmac_sha256, hmac_sha1 or timestamped_hmac.
	Method             string `yaml:"method"`
	SignatureHeader    string `yaml:"signature_header"`
	SignaturePrefix    string `yaml:"signature_prefix,omitempty"`
	TimestampHeader    string `yaml:"timestamp_header,omitempty"`
	TimestampTolerance int64  `yaml:"timestamp_tolerance,omitempty"`
}

// DestinationConfig is one destination of an endpoint.
type DestinationConfig struct {
	Name    string `yaml:"name"`
	URL     string `yaml:"url"`
	Enabled *bool  `yaml:"enabled,omitempty"` // Defaults to true
}

// DestinationList is written either as a list of destinations or, the older
// form, as a map of destination name to URL.
type DestinationList []DestinationConfig

// UnmarshalYAML accepts both forms, keeping the map's order.
func (l *DestinationList) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		var list []DestinationConfig
		if err := node.Decode(&list); err != nil {
			return err
		}
		*l = list
		return nil
	}
	var list []DestinationConfig
	for i := 0; i+1 < len(node.Content); i += 2 {
		var d DestinationConfig
		if err := node.Content[i].Decode(&d.Name); err != nil {
			return err
		}
		if err := node.Content[i+1].Decode(&d.URL); err != nil {
			return fmt.Errorf("destination %q: %w", d.Name, err)
		}
		list = append(list, d)
	}
	*l = list
	return nil
}

// URLFor returns the URL listed for the destination called name, or "".
func (l DestinationList) URLFor(name string) string {
	for _, d := range l {
		if d.Name == name {
			return d.URL
		}
	}
	return ""
}

// ResolvedSecret is the endpoint's signature secret, from secret or
// secret_env, or "" when neither is set.
func (e *EndpointConfig) ResolvedSecret() (string, error) {
	if e.SecretEnv == "" {
		return e.Secret, nil
	}
	v := os.Getenv(e.SecretEnv)
	if v == "" {
		return "", fmt.Errorf("secret_env %s is not set", e.SecretEnv)
	}
	return v, nil
}

// Label names the endpoint in messages.
func (e *EndpointConfig) Label() string {
	switch {
	case e.Name != "" && e.ID != "":
		return fmt.Sprintf("%s (%s)", e.Name, e.ID)
	case e.Name != "":
		return e.Name
	}
	return e.ID
}

// LoadHooklyYAML loads configuration from a YAML file.
func LoadHooklyYAML(path string) (*HooklyConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg HooklyConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Validate checks that required fields are set.
func (c *HooklyConfig) Validate() error {
	if c.EdgeURL == "" {
		return errors.New("edge_url is required")
	}
	if len(c.Endpoints) == 0 && len(c.Proxies) == 0 {
		return errors.New("at least one endpoint (or proxy) is required")
	}

	for i, ep := range c.Endpoints {
		if err := ep.validate(); err != nil {
			return fmt.Errorf("endpoint %d: %w", i, err)
		}
	}

	seen := make(map[string]bool, len(c.Proxies))
	for i, p := range c.Proxies {
		if err := proxy.ValidateUpstream(p.Upstream()); err != nil {
			return fmt.Errorf("proxy %d: %w", i, err)
		}
		if seen[p.Name] {
			return fmt.Errorf("proxy %d: name %q is listed twice", i, p.Name)
		}
		seen[p.Name] = true
	}

	return nil
}

var (
	validProviders = map[string]bool{"stripe": true, "github": true, "telegram": true, "generic": true, "custom": true}
	validMethods   = map[string]bool{"static": true, "hmac_sha256": true, "hmac_sha1": true, "timestamped_hmac": true}
)

func (e *EndpointConfig) validate() error {
	if e.ID == "" && e.Name == "" {
		return errors.New("id or name is required")
	}
	if e.Provider != "" && !validProviders[e.Provider] {
		return fmt.Errorf("unknown provider %q (use stripe, github, telegram, generic or custom)", e.Provider)
	}
	if e.Secret != "" && e.SecretEnv != "" {
		return errors.New("set secret or secret_env, not both")
	}
	if v := e.Verification; v != nil {
		if e.Provider != "custom" {
			return errors.New("verification is only for provider: custom")
		}
		if !validMethods[v.Method] {
			return fmt.Errorf("verification: unknown method %q (use static, hmac_sha256, hmac_sha1 or timestamped_hmac)", v.Method)
		}
		if v.SignatureHeader == "" {
			return errors.New("verification: signature_header is required")
		}
		if v.Method == "timestamped_hmac" && v.TimestampHeader == "" {
			return errors.New("verification: timestamp_header is required for timestamped_hmac")
		}
	} else if e.Provider == "custom" {
		return errors.New("provider custom needs a verification section")
	}
	seen := make(map[string]bool, len(e.Destinations))
	for i, d := range e.Destinations {
		if d.Name == "" || d.URL == "" {
			return fmt.Errorf("destination %d: name and url are required", i)
		}
		if seen[d.Name] {
			return fmt.Errorf("destination %q is listed twice", d.Name)
		}
		seen[d.Name] = true
	}
	return nil
}

// Upstream is the proxy package's view of this entry.
func (p ProxyConfig) Upstream() proxy.Upstream {
	return proxy.Upstream{Name: p.Name, URL: p.URL, Paths: p.Paths}
}

// ProxyUpstreams lists the configured proxies for proxy.NewForwarder.
func (c *HooklyConfig) ProxyUpstreams() []proxy.Upstream {
	ups := make([]proxy.Upstream, 0, len(c.Proxies))
	for _, p := range c.Proxies {
		ups = append(ups, p.Upstream())
	}
	return ups
}

// GetHubID returns the hub ID, auto-generating from hostname if not set.
func (c *HooklyConfig) GetHubID() string {
	if c.HubID != "" {
		return c.HubID
	}
	return generateHubID()
}

// HostHubID is the default hub ID for this machine, derived from its hostname.
func HostHubID() string {
	return generateHubID()
}

// generateHubID creates a hub ID from the machine hostname.
func generateHubID() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	// Sanitize: lowercase, replace spaces/dots with dashes
	hostname = strings.ToLower(hostname)
	hostname = strings.ReplaceAll(hostname, " ", "-")
	hostname = strings.ReplaceAll(hostname, ".", "-")
	return hostname
}

// EndpointIDs returns a list of all endpoint IDs.
func (c *HooklyConfig) EndpointIDs() []string {
	ids := make([]string, len(c.Endpoints))
	for i, ep := range c.Endpoints {
		ids[i] = ep.ID
	}
	return ids
}

// GetDestination returns the URL to forward to for one destination of an endpoint.
//
// An override under `destinations:` for destinationName wins. The legacy
// `destination:` override only applies to the endpoint's primary destination,
// so it can never redirect a second destination's traffic. An edge that
// predates fan-out sends no destination name and has a single destination per
// endpoint, which is treated as primary. Otherwise defaultDest (the URL
// configured on the edge) is returned.
func (c *HooklyConfig) GetDestination(endpointID, destinationName string, primary bool, defaultDest string) string {
	for _, ep := range c.Endpoints {
		if ep.ID != endpointID {
			continue
		}
		if override := ep.Destinations.URLFor(destinationName); destinationName != "" && override != "" {
			return override
		}
		if ep.Destination != "" && (primary || destinationName == "") {
			return ep.Destination
		}
	}
	return defaultDest
}

// ExampleYAML returns an example hookly.yaml configuration.
func ExampleYAML() string {
	return `# Hookly configuration
#
# hookly makes the edge match this file: on start (and with 'hookly sync') it
# creates endpoints and destinations that don't exist, updates what differs,
# and fills in the id and url the edge assigns.
edge_url: "https://hooks.example.com"
# hub_id is optional - auto-generated from hostname if not set
# hub_id: "myapp-dev"

endpoints:
  - name: "stripe"
    provider: stripe              # stripe | github | telegram | generic | custom
    secret_env: STRIPE_WEBHOOK_SECRET
    destinations:
      - name: app
        url: "http://localhost:3000/webhooks/stripe"

  - name: "telegram-bot"
    provider: telegram
    secret_env: BOT_WEBHOOK_SECRET # the setWebhook secret_token
    destinations:                 # the first is the primary
      - name: otto
        url: "http://localhost:8788/telegram"
      - name: schoolboy
        url: "http://localhost:8789/telegram"
    # prune: true                 # remove edge destinations not listed here

# Optional: reverse-proxy local web services through the relay. Each is
# reachable at https://hooks.example.com/p/<hub_id>/<name>/... and only the
# listed path prefixes are forwarded (everything else answers 404). The
# service does its own authentication; the edge verifies nothing.
# proxies:
#   - name: "homeboy"
#     url: "http://127.0.0.1:8790"
#     paths: ["/app/"]
`
}
