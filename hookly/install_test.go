package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hooks.dx314.com/internal/config"
	svc "hooks.dx314.com/internal/service"
)

func writeHooklyYAML(t *testing.T, dir, endpoint string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "hookly.yaml")
	yaml := "edge_url: \"https://hooks.example.com\"\nendpoints:\n  - id: \"" + endpoint + "\"\n"
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheckInstallConflicts(t *testing.T) {
	root := t.TempDir()
	schoolboy := writeHooklyYAML(t, filepath.Join(root, "schoolboy"), "ep_telegram")
	homeboy := writeHooklyYAML(t, filepath.Join(root, "homeboy"), "ep_telegram")
	other := writeHooklyYAML(t, filepath.Join(root, "other"), "ep_other")

	installed := []svc.Installed{
		{Name: "schoolboy", Unit: "hookly-schoolboy", ConfigPath: schoolboy},
	}
	check := func(name, path string) error {
		cfg, err := config.LoadHooklyYAML(path)
		if err != nil {
			t.Fatal(err)
		}
		return checkInstallConflicts(&svc.ServiceConfig{Name: name, ConfigPath: path}, cfg, installed)
	}

	if err := check("homeboy", homeboy); err == nil || !strings.Contains(err.Error(), "already relayed by service hookly-schoolboy") {
		t.Errorf("same endpoint: err = %v, want endpoint conflict", err)
	}
	if err := check("schoolboy", other); err == nil || !strings.Contains(err.Error(), "already runs") {
		t.Errorf("taken name: err = %v, want name conflict", err)
	}
	if err := check("other", other); err != nil {
		t.Errorf("different endpoint: err = %v, want nil", err)
	}
	if err := check("renamed", schoolboy); err != nil {
		t.Errorf("reinstall of same hookly.yaml: err = %v, want nil", err)
	}
}
