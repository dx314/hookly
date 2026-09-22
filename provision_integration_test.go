package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"

	"hooks.dx314.com/internal/api/hookly/v1/hooklyv1connect"
	"hooks.dx314.com/internal/auth"
	"hooks.dx314.com/internal/config"
	"hooks.dx314.com/internal/crypto"
	"hooks.dx314.com/internal/db"
	"hooks.dx314.com/internal/provision"
	"hooks.dx314.com/internal/relay"
	"hooks.dx314.com/internal/server"
	"hooks.dx314.com/internal/service/edge"
)

// provisionEdge runs the edge's API in-process and returns a client
// authenticated as a CLI user, plus the store behind it.
func provisionEdge(t *testing.T) (hooklyv1connect.EdgeServiceClient, *db.Store, *db.SecretManager, *atomic.Int32) {
	t.Helper()
	ctx := context.Background()
	conn, err := db.Open(ctx, filepath.Join(t.TempDir(), "edge.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	store := db.NewStore(conn)

	key, err := crypto.ParseKey("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	if err != nil {
		t.Fatalf("parse key: %v", err)
	}
	secrets := db.NewSecretManager(key)

	tokens := auth.NewTokenManager(store.Queries)
	token, _, err := tokens.GenerateToken(ctx, "user-1", "alex", "test")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	svc := edge.New(store, secrets, relay.NewConnectionManager(), &config.Config{BaseURL: "https://hooks.test"})
	interceptor := server.NewAuthInterceptor(auth.NewSessionManager(store.Queries, false, "/"), tokens)
	path, handler := hooklyv1connect.NewEdgeServiceHandler(svc, connect.WithInterceptors(interceptor))
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	srv := httptest.NewUnstartedServer(mux)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)

	bearer := connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			req.Header().Set("Authorization", "Bearer "+token)
			return next(ctx, req)
		}
	})
	updates := &atomic.Int32{}
	countUpdates := connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if strings.HasSuffix(req.Spec().Procedure, "/UpdateEndpoint") {
				updates.Add(1)
			}
			return next(ctx, req)
		}
	})
	client := hooklyv1connect.NewEdgeServiceClient(srv.Client(), srv.URL, connect.WithGRPC(), connect.WithInterceptors(bearer, countUpdates))
	return client, store, secrets, updates
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
}

func syncYAML(t *testing.T, client hooklyv1connect.EdgeServiceClient, path string, opts provision.Options) (*config.HooklyConfig, *provision.Report) {
	t.Helper()
	cfg, err := config.LoadHooklyYAML(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	report, err := provision.ApplyWith(context.Background(), client, cfg, path, opts)
	if err != nil {
		t.Fatalf("sync: %v (changes %v)", err, report.Changes)
	}
	return cfg, report
}

func TestProvisionFromYAML(t *testing.T) {
	ctx := context.Background()
	client, store, secrets, updates := provisionEdge(t)
	t.Setenv("BOT_SECRET", "s3cret")

	path := filepath.Join(t.TempDir(), "hookly.yaml")
	writeFile(t, path, `edge_url: "https://hooks.test"
# the telegram bot
endpoints:
  - name: bot # comment on the name
    provider: telegram
    secret_env: BOT_SECRET
    destinations:
      - name: schoolboy
        url: http://127.0.0.1:8789/telegram
      - name: homeboy
        url: http://127.0.0.1:8790/telegram
proxies:
  - name: app
    url: http://127.0.0.1:8790
    paths: ["/app/"]
`)

	// Dry run changes nothing
	_, report := syncYAML(t, client, path, provision.Options{DryRun: true})
	if len(report.Changes) != 1 || !strings.HasPrefix(report.Changes[0], "would create endpoint bot") {
		t.Fatalf("dry run changes = %v", report.Changes)
	}
	if eps, _ := store.ListEndpoints(ctx, db.ListEndpointsParams{UserID: "user-1", Limit: 10}); len(eps) != 0 {
		t.Fatalf("dry run created %d endpoints", len(eps))
	}

	// Creates the endpoint with both destinations and records its id and URL
	cfg, _ := syncYAML(t, client, path, provision.Options{})
	id := cfg.Endpoints[0].ID
	if id == "" {
		t.Fatal("id not filled in")
	}
	ep, err := store.GetEndpoint(ctx, db.GetEndpointParams{ID: id, UserID: "user-1"})
	if err != nil {
		t.Fatalf("endpoint not created: %v", err)
	}
	if ep.ProviderType != "telegram" {
		t.Errorf("provider = %s, want telegram", ep.ProviderType)
	}
	if secret, _ := secrets.DecryptSecret(ep.SignatureSecretEncrypted); secret != "s3cret" {
		t.Errorf("secret = %q, want s3cret", secret)
	}
	dests, _ := store.ListDestinationsByEndpoint(ctx, id)
	if len(dests) != 2 || dests[0].Name != "schoolboy" || dests[1].Name != "homeboy" {
		t.Fatalf("destinations = %+v", dests)
	}

	data, _ := os.ReadFile(path)
	text := string(data)
	for _, want := range []string{
		`id: "` + id + `"`,
		`url: "https://hooks.test/h/` + id + `"`,
		"# the telegram bot",
		"# comment on the name",
		`paths: ["/app/"]`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("hookly.yaml lacks %q:\n%s", want, text)
		}
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640", info.Mode().Perm())
	}

	// In sync: nothing to do, not even the secret (its fingerprint matches), file untouched
	before, _ := os.ReadFile(path)
	updates.Store(0)
	if _, report := syncYAML(t, client, path, provision.Options{}); len(report.Changes) != 0 || len(report.Fields) != 0 {
		t.Errorf("second sync changed %v / %v", report.Changes, report.Fields)
	}
	if n := updates.Load(); n != 0 {
		t.Errorf("second sync made %d UpdateEndpoint calls, want 0", n)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Error("second sync rewrote hookly.yaml")
	}

	// A changed secret is detected by fingerprint and sent
	t.Setenv("BOT_SECRET", "rotated")
	if _, report := syncYAML(t, client, path, provision.Options{}); len(report.Changes) != 1 || report.Changes[0] != "update endpoint bot secret" {
		t.Errorf("secret rotation changes = %v", report.Changes)
	}
	ep, _ = store.GetEndpoint(ctx, db.GetEndpointParams{ID: id, UserID: "user-1"})
	if secret, _ := secrets.DecryptSecret(ep.SignatureSecretEncrypted); secret != "rotated" {
		t.Errorf("secret = %q, want rotated", secret)
	}

	// Edits flow to the edge: URL change, disable, new destination, provider
	edited := strings.NewReplacer(
		"provider: telegram", "provider: github",
		"http://127.0.0.1:8789/telegram", "http://127.0.0.1:9999/telegram",
		"        url: http://127.0.0.1:8790/telegram\n", "        url: http://127.0.0.1:8790/telegram\n        enabled: false\n      - name: otto\n        url: http://127.0.0.1:8788/telegram\n",
	).Replace(string(before))
	writeFile(t, path, edited)
	_, report = syncYAML(t, client, path, provision.Options{})
	if len(report.Changes) != 4 {
		t.Errorf("changes = %v, want provider, url, enabled, add", report.Changes)
	}
	ep, _ = store.GetEndpoint(ctx, db.GetEndpointParams{ID: id, UserID: "user-1"})
	if ep.ProviderType != "github" {
		t.Errorf("provider = %s, want github", ep.ProviderType)
	}
	dests, _ = store.ListDestinationsByEndpoint(ctx, id)
	got := map[string]db.Destination{}
	for _, d := range dests {
		got[d.Name] = d
	}
	if got["schoolboy"].Url != "http://127.0.0.1:9999/telegram" || got["homeboy"].Enabled != 0 || got["otto"].Url != "http://127.0.0.1:8788/telegram" {
		t.Errorf("destinations = %+v", dests)
	}

	// A destination only on the edge is left alone, unless prune is set
	if _, err := store.AddDestination(ctx, "user-1", id, db.DestinationSpec{Name: "extra", URL: "http://x", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, report := syncYAML(t, client, path, provision.Options{}); len(report.Notes) != 1 || !strings.Contains(report.Notes[0], "extra") {
		t.Errorf("notes = %v, want one about extra", report.Notes)
	}
	writeFile(t, path, strings.Replace(edited, "    provider: github", "    prune: true\n    provider: github", 1))
	if _, report := syncYAML(t, client, path, provision.Options{}); len(report.Changes) != 1 || report.Changes[0] != "remove destination extra from bot" {
		t.Errorf("prune changes = %v", report.Changes)
	}

	// Another hookly.yaml naming the same endpoint adopts it instead of creating one
	other := filepath.Join(t.TempDir(), "hookly.yaml")
	writeFile(t, other, "edge_url: \"https://hooks.test\"\nendpoints:\n  - name: bot\n")
	cfg, report = syncYAML(t, client, other, provision.Options{})
	if cfg.Endpoints[0].ID != id || len(report.Changes) != 0 {
		t.Errorf("adopt: id %s changes %v, want %s and none", cfg.Endpoints[0].ID, report.Changes, id)
	}

	// An id that doesn't exist is an error, not a silent new endpoint
	writeFile(t, other, "edge_url: \"https://hooks.test\"\nendpoints:\n  - id: ep_gone\n    name: bot\n")
	cfg, _ = config.LoadHooklyYAML(other)
	_, err = provision.ApplyWith(ctx, client, cfg, other, provision.Options{})
	if err == nil || !provision.IsPermanent(err) || !strings.Contains(err.Error(), "ep_gone") {
		t.Errorf("unknown id: err = %v, want permanent error naming it", err)
	}

	// An endpoint that doesn't exist needs destinations to be created
	writeFile(t, other, "edge_url: \"https://hooks.test\"\nendpoints:\n  - name: new-one\n")
	cfg, _ = config.LoadHooklyYAML(other)
	if _, err := provision.ApplyWith(ctx, client, cfg, other, provision.Options{}); err == nil || !provision.IsPermanent(err) {
		t.Errorf("no destinations: err = %v, want permanent error", err)
	}

}
