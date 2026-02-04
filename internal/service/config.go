package service

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

// ServiceConfig holds configuration for the service.
type ServiceConfig struct {
	ConfigPath  string // Path to hookly.yaml
	WorkingDir  string // Working directory for the service
	LogPath     string // Path for log output (macOS only)
	UserService bool   // Install as user service (no sudo)
}

// DefaultServiceConfig returns platform-appropriate default configuration.
func DefaultServiceConfig(userService bool) *ServiceConfig {
	cfg := &ServiceConfig{
		UserService: userService,
	}

	if userService {
		cfg.ConfigPath = userConfigPath()
		cfg.LogPath = userLogPath()
	} else {
		cfg.ConfigPath = systemConfigPath()
		cfg.LogPath = systemLogPath()
	}

	return cfg
}

// userConfigPath returns the default config path for user services.
func userConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "hookly.yaml"
	}
	return filepath.Join(home, ".config", "hookly", "hookly.yaml")
}

// systemConfigPath returns the default config path for system services.
func systemConfigPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("ProgramData"), "hookly", "hookly.yaml")
	}
	return "/etc/hookly/hookly.yaml"
}

// userLogPath returns the default log path for user services.
func userLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	if runtime.GOOS == "darwin" {
		return filepath.Join(home, ".local", "share", "hookly", "hookly.log")
	}
	// Linux uses journalctl, no log path needed
	return ""
}

// systemLogPath returns the default log path for system services.
func systemLogPath() string {
	if runtime.GOOS == "darwin" {
		return "/var/log/hookly/hookly.log"
	}
	// Linux uses journalctl, no log path needed
	return ""
}

// Validate checks that the service configuration is valid.
func (c *ServiceConfig) Validate() error {
	if c.ConfigPath == "" {
		return errors.New("config path is required")
	}

	// Check if config file exists
	if _, err := os.Stat(c.ConfigPath); os.IsNotExist(err) {
		return errors.New("config file not found: " + c.ConfigPath + "\n\nRun 'hookly init' to create a configuration file")
	}

	return nil
}

// ValidateForInstall performs additional validation for service installation.
func (c *ServiceConfig) ValidateForInstall() error {
	if err := c.Validate(); err != nil {
		return err
	}

	// Check if running from a temporary location (go run)
	exe, err := os.Executable()
	if err != nil {
		return errors.New("failed to determine executable path")
	}

	// Check for common temp/build locations
	if isTemporaryPath(exe) {
		return errors.New("cannot install service from temporary location\n\nInstall hookly first with: go install hooks.dx314.com/hookly@latest")
	}

	return nil
}

// isTemporaryPath checks if the path appears to be a temporary location.
func isTemporaryPath(path string) bool {
	tempIndicators := []string{
		"/go-build",
		"/tmp/go-build",
		"\\Temp\\go-build",
		"/var/folders/", // macOS temp
	}

	for _, indicator := range tempIndicators {
		if contains(path, indicator) {
			return true
		}
	}
	return false
}

// contains checks if s contains substr (simple implementation to avoid strings import).
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
