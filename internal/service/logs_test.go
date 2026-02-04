package service

import (
	"runtime"
	"strings"
	"testing"
)

func TestGetLogPath(t *testing.T) {
	t.Run("user service", func(t *testing.T) {
		path := GetLogPath(true)

		if runtime.GOOS == "darwin" {
			if !strings.Contains(path, ".local/share/hookly") {
				t.Errorf("expected macOS user log path, got %s", path)
			}
		} else if runtime.GOOS == "linux" {
			if !strings.Contains(path, "journalctl --user") {
				t.Errorf("expected Linux journalctl command, got %s", path)
			}
		}
	})

	t.Run("system service", func(t *testing.T) {
		path := GetLogPath(false)

		if runtime.GOOS == "darwin" {
			if !strings.Contains(path, "/var/log/hookly") {
				t.Errorf("expected macOS system log path, got %s", path)
			}
		} else if runtime.GOOS == "linux" {
			if !strings.Contains(path, "journalctl -u") {
				t.Errorf("expected Linux journalctl command, got %s", path)
			}
			if strings.Contains(path, "--user") {
				t.Error("system service should not have --user flag")
			}
		}
	})
}

func TestLogsConfig(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		cfg := &LogsConfig{}

		if cfg.Follow {
			t.Error("Follow should default to false")
		}
		if cfg.Lines != 0 {
			t.Errorf("Lines should default to 0, got %d", cfg.Lines)
		}
		if cfg.UserService {
			t.Error("UserService should default to false")
		}
	})

	t.Run("with values", func(t *testing.T) {
		cfg := &LogsConfig{
			Follow:      true,
			Lines:       100,
			UserService: true,
		}

		if !cfg.Follow {
			t.Error("Follow should be true")
		}
		if cfg.Lines != 100 {
			t.Errorf("Lines should be 100, got %d", cfg.Lines)
		}
		if !cfg.UserService {
			t.Error("UserService should be true")
		}
	})
}

func TestUserLogPath(t *testing.T) {
	path := userLogPath()

	if runtime.GOOS == "darwin" {
		if !strings.Contains(path, ".local/share/hookly/hookly.log") {
			t.Errorf("unexpected macOS user log path: %s", path)
		}
	} else if runtime.GOOS == "linux" {
		// Linux uses journalctl, so path should be empty
		if path != "" {
			t.Errorf("expected empty path for Linux, got %s", path)
		}
	}
}

func TestSystemLogPath(t *testing.T) {
	path := systemLogPath()

	if runtime.GOOS == "darwin" {
		if path != "/var/log/hookly/hookly.log" {
			t.Errorf("unexpected macOS system log path: %s", path)
		}
	} else if runtime.GOOS == "linux" {
		// Linux uses journalctl, so path should be empty
		if path != "" {
			t.Errorf("expected empty path for Linux, got %s", path)
		}
	}
}
