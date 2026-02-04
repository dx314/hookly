package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// HooklyConfig holds configuration for the hookly CLI.
type HooklyConfig struct {
	EdgeURL   string           `yaml:"edge_url"`
	Secret    string           `yaml:"secret"`
	HubID     string           `yaml:"hub_id"`
	Endpoints []EndpointConfig `yaml:"endpoints"`
}

// EndpointConfig defines an endpoint this hub handles.
type EndpointConfig struct {
	ID          string `yaml:"id"`
	Destination string `yaml:"destination,omitempty"` // Optional override
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
	if c.Secret == "" {
		return errors.New("secret is required")
	}
	if c.HubID == "" {
		return errors.New("hub_id is required")
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

// EndpointIDs returns a list of all endpoint IDs.
func (c *HooklyConfig) EndpointIDs() []string {
	ids := make([]string, len(c.Endpoints))
	for i, ep := range c.Endpoints {
		ids[i] = ep.ID
	}
	return ids
}

// GetDestination returns the destination URL for an endpoint.
// If the endpoint has a destination override, it's returned.
// Otherwise, defaultDest is returned.
func (c *HooklyConfig) GetDestination(endpointID, defaultDest string) string {
	for _, ep := range c.Endpoints {
		if ep.ID == endpointID && ep.Destination != "" {
			return ep.Destination
		}
	}
	return defaultDest
}

// ExampleYAML returns an example hookly.yaml configuration.
func ExampleYAML() string {
	return `# Hookly configuration
edge_url: "https://hooks.example.com"
secret: "your-home-hub-secret"
hub_id: "myapp-dev"

endpoints:
  - id: "ep_abc123"
    destination: "http://localhost:3000/webhooks/stripe"
  - id: "ep_def456"
    # Uses edge-configured destination (no override)
`
}
