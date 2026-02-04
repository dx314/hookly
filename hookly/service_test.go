package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMakeAbsolute(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home directory: %v", err)
	}

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "absolute path unchanged",
			path:     "/etc/hookly/hookly.yaml",
			expected: "/etc/hookly/hookly.yaml",
		},
		{
			name:     "relative path made absolute",
			path:     "hookly.yaml",
			expected: wd + "/hookly.yaml",
		},
		{
			name:     "tilde path expanded",
			path:     "~/.config/hookly/hookly.yaml",
			expected: home + "/.config/hookly/hookly.yaml",
		},
		{
			name:     "relative with subdirectory",
			path:     "config/hookly.yaml",
			expected: wd + "/config/hookly.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := makeAbsolute(tt.path)
			if err != nil {
				t.Fatalf("makeAbsolute(%q) returned error: %v", tt.path, err)
			}
			if result != tt.expected {
				t.Errorf("makeAbsolute(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIsPermissionError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "permission denied",
			err:      os.ErrPermission,
			expected: true,
		},
		{
			name:     "other error",
			err:      os.ErrNotExist,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPermissionError(tt.err)
			if result != tt.expected {
				t.Errorf("isPermissionError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestIsNotInstalledError(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		expected bool
	}{
		{
			name:     "not installed message",
			errMsg:   "service not installed",
			expected: true,
		},
		{
			name:     "does not exist message",
			errMsg:   "unit does not exist",
			expected: true,
		},
		{
			name:     "could not be found message",
			errMsg:   "service could not be found",
			expected: true,
		},
		{
			name:     "other error",
			errMsg:   "connection refused",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &testError{msg: tt.errMsg}
			result := isNotInstalledError(err)
			if result != tt.expected {
				t.Errorf("isNotInstalledError(%q) = %v, want %v", tt.errMsg, result, tt.expected)
			}
		})
	}
}

func TestContainsInService(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"permission denied", "denied", true},
		{"access denied", "denied", true},
		{"operation not permitted", "not permitted", true},
		{"some other error", "denied", false},
		{"", "denied", false},
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

func TestBuildServiceConfig(t *testing.T) {
	// Create temp config for testing
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

	// Note: Testing buildServiceConfig requires a cli.Context which is complex to mock
	// These tests focus on the helper functions it uses
}

func TestIsServiceMode(t *testing.T) {
	// Save original args
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	t.Run("service mode flag present", func(t *testing.T) {
		os.Args = []string{"hookly", "--service-mode", "--config", "/etc/hookly/hookly.yaml"}
		if !isServiceMode() {
			t.Error("expected isServiceMode() to return true")
		}
	})

	t.Run("service mode flag absent", func(t *testing.T) {
		os.Args = []string{"hookly", "status"}
		if isServiceMode() {
			t.Error("expected isServiceMode() to return false")
		}
	})
}

func TestGetServiceConfigPath(t *testing.T) {
	// Save original args
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	t.Run("config flag present", func(t *testing.T) {
		os.Args = []string{"hookly", "--service-mode", "--config", "/custom/hookly.yaml"}
		path := getServiceConfigPath()
		if path != "/custom/hookly.yaml" {
			t.Errorf("expected /custom/hookly.yaml, got %s", path)
		}
	})

	t.Run("config flag absent", func(t *testing.T) {
		os.Args = []string{"hookly", "--service-mode"}
		path := getServiceConfigPath()
		if path != "/etc/hookly/hookly.yaml" {
			t.Errorf("expected default path /etc/hookly/hookly.yaml, got %s", path)
		}
	})
}

// testError is a simple error type for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
