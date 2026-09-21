package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The service authenticates with the login saved by `hookly login`; without
// one it must refuse to start with a clear message instead of connecting with
// an empty token.
func TestStartRequiresLogin(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config")) // no credentials here
	t.Setenv("HOME", dir)

	configPath := filepath.Join(dir, "hookly.yaml")
	yaml := "edge_url: \"https://hooks.example.com\"\nendpoints:\n  - id: \"ep_test\"\n"
	if err := os.WriteFile(configPath, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	p := &Program{cfg: &ServiceConfig{ConfigPath: configPath}}
	err := p.Start(nil)
	if err == nil {
		_ = p.Stop(nil)
		t.Fatal("service started without a saved login")
	}
	if !strings.Contains(err.Error(), "not logged in") {
		t.Errorf("error = %q, want it to say not logged in", err)
	}
}
