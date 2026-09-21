package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// ServiceConfig holds configuration for the service.
type ServiceConfig struct {
	Name        string // Instance name; "" is the original single "hookly" service
	ConfigPath  string // Path to hookly.yaml
	WorkingDir  string // Working directory for the service
	UserService bool   // Install as user service (no sudo)
}

// UnitName is the service manager's name for this instance: "hookly" for the
// unnamed service, "hookly-<name>" for a named one.
func (c *ServiceConfig) UnitName() string {
	return UnitName(c.Name)
}

// UnitName returns the service manager's name for the instance called name.
func UnitName(name string) string {
	if name == "" {
		return serviceName
	}
	return serviceName + "-" + name
}

var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// ValidateName checks that name is usable in a unit name and a hub ID.
func ValidateName(name string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("invalid service name %q: use lowercase letters, digits and dashes", name)
	}
	return nil
}

// SanitizeName turns s (typically a directory name) into a valid service
// name, or "" when nothing usable is left.
func SanitizeName(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	name := strings.TrimRight(b.String(), "-")
	if len(name) > 63 {
		name = strings.TrimRight(name[:63], "-")
	}
	return name
}

// DefaultName is the service name for a hookly.yaml: its directory's name,
// so ~/homeboy/hookly.yaml becomes "homeboy" (unit "hookly-homeboy").
func DefaultName(configPath string) string {
	if name := SanitizeName(filepath.Base(filepath.Dir(configPath))); name != "" {
		return name
	}
	return "relay"
}

// DefaultServiceConfig returns platform-appropriate default configuration.
func DefaultServiceConfig(userService bool) *ServiceConfig {
	cfg := &ServiceConfig{
		UserService: userService,
	}

	if userService {
		cfg.ConfigPath = userConfigPath()
	} else {
		cfg.ConfigPath = systemConfigPath()
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

// logDir is where launchd writes the service's output on macOS. Linux uses
// journalctl, so it is "" there.
func logDir(userService bool) string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	if !userService {
		return "/var/log/hookly"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "hookly")
}

// LogPath is the file the service logs to on macOS (launchd names it
// <unit>.out.log), or "" on Linux, where logs go to journald.
func (c *ServiceConfig) LogPath() string {
	dir := logDir(c.UserService)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, c.UnitName()+".out.log")
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
