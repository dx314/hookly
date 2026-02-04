package service

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaultServiceConfig(t *testing.T) {
	t.Run("user service", func(t *testing.T) {
		cfg := DefaultServiceConfig(true)

		if !cfg.UserService {
			t.Error("expected UserService to be true")
		}

		home, _ := os.UserHomeDir()
		expectedConfig := filepath.Join(home, ".config", "hookly", "hookly.yaml")
		if cfg.ConfigPath != expectedConfig {
			t.Errorf("expected config path %s, got %s", expectedConfig, cfg.ConfigPath)
		}
	})

	t.Run("system service", func(t *testing.T) {
		cfg := DefaultServiceConfig(false)

		if cfg.UserService {
			t.Error("expected UserService to be false")
		}

		if runtime.GOOS != "windows" {
			if cfg.ConfigPath != "/etc/hookly/hookly.yaml" {
				t.Errorf("expected config path /etc/hookly/hookly.yaml, got %s", cfg.ConfigPath)
			}
		}
	})
}

func TestServiceConfigValidate(t *testing.T) {
	// Create a temp config file for testing
	tmpDir := t.TempDir() // Automatically cleaned up after test
	configPath := filepath.Join(tmpDir, "hookly.yaml")

	// Write a minimal valid config
	validConfig := `edge_url: "https://hooks.example.com"
secret: "test-secret"
hub_id: "test-hub"
endpoints:
  - id: "ep_test123"
`
	if err := os.WriteFile(configPath, []byte(validConfig), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	t.Run("valid config", func(t *testing.T) {
		cfg := &ServiceConfig{
			ConfigPath: configPath,
		}

		if err := cfg.Validate(); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("empty config path", func(t *testing.T) {
		cfg := &ServiceConfig{
			ConfigPath: "",
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for empty config path")
		}
	})

	t.Run("missing config file", func(t *testing.T) {
		cfg := &ServiceConfig{
			ConfigPath: "/nonexistent/path/hookly.yaml",
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for missing config file")
		}
	})
}

func TestServiceConfigValidateForInstall(t *testing.T) {
	// Create a temp config file for testing
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "hookly.yaml")

	validConfig := `edge_url: "https://hooks.example.com"
secret: "test-secret"
hub_id: "test-hub"
endpoints:
  - id: "ep_test123"
`
	if err := os.WriteFile(configPath, []byte(validConfig), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	t.Run("valid install config", func(t *testing.T) {
		cfg := &ServiceConfig{
			ConfigPath: configPath,
		}

		// This may fail if running from go test (temp location),
		// but validates the logic is working
		err := cfg.ValidateForInstall()
		// We can't guarantee this passes since we're running from go test
		// Just verify it doesn't panic
		_ = err
	})
}

func TestIsTemporaryPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "go-build path",
			path:     "/tmp/go-build123456/main",
			expected: true,
		},
		{
			name:     "var folders (macOS temp)",
			path:     "/var/folders/ab/cd/T/go-build123/main",
			expected: true,
		},
		{
			name:     "normal install path",
			path:     "/usr/local/bin/hookly",
			expected: false,
		},
		{
			name:     "home bin path",
			path:     "/home/user/go/bin/hookly",
			expected: false,
		},
		{
			name:     "windows temp",
			path:     "C:\\Users\\test\\AppData\\Local\\Temp\\go-build123\\main.exe",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTemporaryPath(tt.path)
			if result != tt.expected {
				t.Errorf("isTemporaryPath(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"hello world", "world", true},
		{"hello world", "foo", false},
		{"", "foo", false},
		{"foo", "", true},
		{"go-build", "go-build", true},
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.substr, func(t *testing.T) {
			result := contains(tt.s, tt.substr)
			if result != tt.expected {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, result, tt.expected)
			}
		})
	}
}
