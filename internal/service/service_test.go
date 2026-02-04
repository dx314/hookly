package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kardianos/service"
)

func TestNewService(t *testing.T) {
	// Create a temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "hookly.yaml")

	validConfig := `edge_url: "https://hooks.example.com"
hub_id: "test-hub"
endpoints:
  - id: "ep_test123"
`
	if err := os.WriteFile(configPath, []byte(validConfig), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	t.Run("creates service with user config", func(t *testing.T) {
		cfg := &ServiceConfig{
			ConfigPath:  configPath,
			UserService: true,
		}

		svc, err := NewService(cfg)
		if err != nil {
			// On systems without a service manager, this may fail
			// which is acceptable for unit tests
			if err == service.ErrNoServiceSystemDetected {
				t.Skip("no service system detected")
			}
			t.Fatalf("NewService failed: %v", err)
		}

		if svc == nil {
			t.Error("expected non-nil service")
		}
	})

	t.Run("creates service with system config", func(t *testing.T) {
		cfg := &ServiceConfig{
			ConfigPath:  configPath,
			UserService: false,
		}

		svc, err := NewService(cfg)
		if err != nil {
			if err == service.ErrNoServiceSystemDetected {
				t.Skip("no service system detected")
			}
			t.Fatalf("NewService failed: %v", err)
		}

		if svc == nil {
			t.Error("expected non-nil service")
		}
	})
}

func TestStatusString(t *testing.T) {
	tests := []struct {
		status   service.Status
		expected string
	}{
		{service.StatusRunning, "running"},
		{service.StatusStopped, "stopped"},
		{service.StatusUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := StatusString(tt.status)
			if result != tt.expected {
				t.Errorf("StatusString(%v) = %q, want %q", tt.status, result, tt.expected)
			}
		})
	}
}

func TestProgram(t *testing.T) {
	// Create a temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "hookly.yaml")

	// Write an invalid config (missing endpoints) to test error handling
	invalidConfig := `edge_url: "https://hooks.example.com"
hub_id: "test-hub"
endpoints: []
`
	if err := os.WriteFile(configPath, []byte(invalidConfig), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	t.Run("start fails with invalid config", func(t *testing.T) {
		cfg := &ServiceConfig{
			ConfigPath: configPath,
		}

		prg := &Program{cfg: cfg}

		// Start should fail because config is invalid (no endpoints)
		err := prg.Start(nil)
		if err == nil {
			// If it didn't fail, make sure to stop it
			prg.Stop(nil)
			t.Error("expected error for invalid config")
		}
	})

	t.Run("stop handles nil cancel gracefully", func(t *testing.T) {
		cfg := &ServiceConfig{
			ConfigPath: configPath,
		}

		prg := &Program{cfg: cfg}

		// Stop should not panic even if Start was never called
		err := prg.Stop(nil)
		if err != nil {
			t.Errorf("Stop returned unexpected error: %v", err)
		}
	})
}

func TestControlServiceInvalidAction(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "hookly.yaml")

	validConfig := `edge_url: "https://hooks.example.com"
hub_id: "test-hub"
endpoints:
  - id: "ep_test123"
`
	if err := os.WriteFile(configPath, []byte(validConfig), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg := &ServiceConfig{
		ConfigPath: configPath,
	}

	err := ControlService(cfg, "invalid-action")
	if err == nil {
		t.Error("expected error for invalid action")
	}
}
