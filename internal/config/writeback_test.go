package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDestinationListForms(t *testing.T) {
	var ep EndpointConfig
	src := "id: a\ndestinations:\n  zed: http://z\n  alpha: http://a\n"
	if err := yaml.Unmarshal([]byte(src), &ep); err != nil {
		t.Fatal(err)
	}
	if len(ep.Destinations) != 2 || ep.Destinations[0].Name != "zed" || ep.Destinations[1].URL != "http://a" {
		t.Errorf("map form = %+v, want zed then alpha", ep.Destinations)
	}

	src = "id: a\ndestinations:\n  - name: one\n    url: http://1\n    enabled: false\n"
	ep = EndpointConfig{}
	if err := yaml.Unmarshal([]byte(src), &ep); err != nil {
		t.Fatal(err)
	}
	if len(ep.Destinations) != 1 || ep.Destinations[0].Enabled == nil || *ep.Destinations[0].Enabled {
		t.Errorf("list form = %+v", ep.Destinations)
	}
	if ep.Destinations.URLFor("one") != "http://1" || ep.Destinations.URLFor("two") != "" {
		t.Error("URLFor")
	}
}

func TestEndpointValidate(t *testing.T) {
	cases := map[string]string{
		"id: a":                             "",
		"name: a":                           "",
		"provider: x":                       "id or name is required",
		"name: a\nprovider: nope":           "unknown provider",
		"name: a\nsecret: x\nsecret_env: Y": "not both",
		"name: a\nprovider: custom":         "needs a verification section",
		"name: a\nprovider: custom\nverification: {method: hmac_sha256}":                               "signature_header is required",
		"name: a\nprovider: custom\nverification: {method: timestamped_hmac, signature_header: X-Sig}": "timestamp_header is required",
		"name: a\nprovider: github\nverification: {method: static, signature_header: X}":               "only for provider: custom",
		"name: a\ndestinations: [{name: x, url: http://1}, {name: x, url: http://2}]":                  "listed twice",
		"name: a\ndestinations: [{name: x}]":                                                           "name and url are required",
	}
	for src, want := range cases {
		var ep EndpointConfig
		if err := yaml.Unmarshal([]byte(src), &ep); err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		err := ep.validate()
		if want == "" && err != nil {
			t.Errorf("%q: unexpected error %v", src, err)
		}
		if want != "" && (err == nil || !strings.Contains(err.Error(), want)) {
			t.Errorf("%q: err = %v, want %q", src, err, want)
		}
	}
}

func TestResolvedSecret(t *testing.T) {
	t.Setenv("HOOKLY_TEST_SECRET", "from-env")
	if s, _ := (&EndpointConfig{Secret: "inline"}).ResolvedSecret(); s != "inline" {
		t.Errorf("inline = %q", s)
	}
	if s, _ := (&EndpointConfig{SecretEnv: "HOOKLY_TEST_SECRET"}).ResolvedSecret(); s != "from-env" {
		t.Errorf("env = %q", s)
	}
	if _, err := (&EndpointConfig{SecretEnv: "HOOKLY_TEST_UNSET"}).ResolvedSecret(); err == nil {
		t.Error("unset env: want error")
	}
}

func TestSetEndpointFields(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.yaml")
	link := filepath.Join(dir, "hookly.yaml")
	src := "edge_url: \"https://e\"\nendpoints:\n  - provider: telegram # no name\n    destinations: {a: \"http://a\"}\n  - name: second\n    id: old\n"
	if err := os.WriteFile(real, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	err := SetEndpointFields(link, []EndpointField{
		{Endpoint: 0, Key: "url", Value: "https://e/h/new"},
		{Endpoint: 0, Key: "id", Value: "new"},
		{Endpoint: 1, Key: "id", Value: "replaced"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink was replaced by a file")
	}
	data, _ := os.ReadFile(real)
	want := "edge_url: \"https://e\"\nendpoints:\n  - id: \"new\"\n    url: \"https://e/h/new\"\n    provider: telegram # no name\n    destinations: {a: \"http://a\"}\n  - name: second\n    id: replaced\n"
	if string(data) != want {
		t.Errorf("got:\n%s\nwant:\n%s", data, want)
	}
	if err := SetEndpointFields(link, []EndpointField{{Endpoint: 5, Key: "id", Value: "x"}}); err == nil {
		t.Error("out of range endpoint: want error")
	}
	cfg, err := LoadHooklyYAML(link)
	if err != nil || cfg.Endpoints[0].ID != "new" || cfg.Endpoints[1].ID != "replaced" {
		t.Errorf("reload: %v %+v", err, cfg)
	}
}
