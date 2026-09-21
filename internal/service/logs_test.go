package service

import (
	"runtime"
	"strings"
	"testing"
)

func TestLogsHint(t *testing.T) {
	cases := []struct {
		cfg    *ServiceConfig
		darwin string // suffix
		linux  string
	}{
		{&ServiceConfig{UserService: true}, ".local/share/hookly/hookly.out.log", "journalctl --user -u hookly"},
		{&ServiceConfig{UserService: true, Name: "homeboy"}, ".local/share/hookly/hookly-homeboy.out.log", "journalctl --user -u hookly-homeboy"},
		{&ServiceConfig{Name: "homeboy"}, "/var/log/hookly/hookly-homeboy.out.log", "journalctl -u hookly-homeboy"},
	}
	for _, tc := range cases {
		hint := LogsHint(tc.cfg)
		switch runtime.GOOS {
		case "darwin":
			if !strings.HasSuffix(hint, tc.darwin) {
				t.Errorf("LogsHint(%+v) = %q, want suffix %q", tc.cfg, hint, tc.darwin)
			}
		case "linux":
			if hint != tc.linux {
				t.Errorf("LogsHint(%+v) = %q, want %q", tc.cfg, hint, tc.linux)
			}
		}
	}
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
