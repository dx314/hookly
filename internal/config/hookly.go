package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// HooklyConfig holds configuration for the hookly CLI.
type HooklyConfig struct {
	EdgeURL   string           `yaml:"edge_url"`
	HubID     string           `yaml:"hub_id,omitempty"` // Optional, auto-generated from hostname if empty
	Endpoints []EndpointConfig `yaml:"endpoints"`
	// Token is loaded from credentials, not from YAML
	Token string `yaml:"-"`
}

// EndpointConfig defines an endpoint this hub handles.
type EndpointConfig struct {
	ID string `yaml:"id"`
	// Optional override for the endpoint's primary (first) destination.
	Destination string `yaml:"destination,omitempty"`
	// Optional overrides per destination, keyed by the destination's name on the edge.
	Destinations map[string]string `yaml:"destinations,omitempty"`
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
	if len(c.Endpoints) == 0 {
		return errors.New("at least one endpoint is required")
	}

	for i, ep := range c.Endpoints {
		if ep.ID == "" {
			return fmt.Errorf("endpoint %d: id is required", i)
		}
	}

	return nil
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
		if override := ep.Destinations[destinationName]; destinationName != "" && override != "" {
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
edge_url: "https://hooks.example.com"
# hub_id is optional - auto-generated from hostname if not set
# hub_id: "myapp-dev"

endpoints:
  - id: "ep_abc123"
    destination: "http://localhost:3000/webhooks/stripe"
  - id: "ep_def456"
    # Uses edge-configured destinations (no override)
  - id: "ep_ghi789"
    # Endpoint with several destinations: override by destination name.
    # "destination" above only ever overrides the primary (first) destination.
    destinations:
      otto: "http://localhost:8788/telegram"
      schoolboy: "http://localhost:8789/telegram"
`
}
