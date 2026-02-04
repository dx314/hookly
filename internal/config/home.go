package config

import (
	"errors"
	"os"

	"github.com/joho/godotenv"
)

// HomeConfig holds configuration for the home-hub service.
type HomeConfig struct {
	EdgeURL       string // https://hooks.dx314.com
	HomeHubSecret string // Pre-shared secret
	HubID         string // Identifier for this hub
}

// LoadHome loads home-hub configuration from environment variables.
func LoadHome() (*HomeConfig, error) {
	// Load .env file if present (ignore errors)
	_ = godotenv.Load()

	cfg := &HomeConfig{}

	cfg.EdgeURL = os.Getenv("EDGE_URL")
	if cfg.EdgeURL == "" {
		return nil, errors.New("EDGE_URL is required")
	}

	cfg.HomeHubSecret = os.Getenv("HOME_HUB_SECRET")
	if cfg.HomeHubSecret == "" {
		return nil, errors.New("HOME_HUB_SECRET is required")
	}

	cfg.HubID = os.Getenv("HUB_ID")
	if cfg.HubID == "" {
		cfg.HubID = "home-hub-1" // Default hub ID
	}

	return cfg, nil
}
