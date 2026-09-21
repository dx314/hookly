package db_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"hooks.dx314.com/internal/db"
)

// Regression: UpdateEndpoint mixed sqlc.narg() with bare "?" placeholders.
// SQLite numbers a bare "?" after the highest numbered parameter, so the query
// wanted 9 arguments but got 7 and every endpoint update (edit, mute) failed
// with "not enough args to execute query".
func TestUpdateEndpoint(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer conn.Close()
	queries := db.New(conn)

	if _, err := queries.CreateEndpoint(ctx, db.CreateEndpointParams{
		ID: "ep", UserID: "user-1", Name: "before", ProviderType: "generic", DestinationUrl: "http://localhost/a",
	}); err != nil {
		t.Fatalf("create endpoint: %v", err)
	}

	// Mute only: other fields keep their values
	ep, err := queries.UpdateEndpoint(ctx, db.UpdateEndpointParams{
		ID: "ep", UserID: "user-1", Muted: sql.NullInt64{Int64: 1, Valid: true},
	})
	if err != nil {
		t.Fatalf("mute endpoint: %v", err)
	}
	if ep.Muted != 1 || ep.Name != "before" || ep.DestinationUrl != "http://localhost/a" {
		t.Errorf("after mute: %+v", ep)
	}

	// Rename and re-point
	ep, err = queries.UpdateEndpoint(ctx, db.UpdateEndpointParams{
		ID: "ep", UserID: "user-1",
		Name:           sql.NullString{String: "after", Valid: true},
		DestinationUrl: sql.NullString{String: "http://localhost/b", Valid: true},
	})
	if err != nil {
		t.Fatalf("update endpoint: %v", err)
	}
	if ep.Name != "after" || ep.DestinationUrl != "http://localhost/b" || ep.Muted != 1 {
		t.Errorf("after update: %+v", ep)
	}

	// Another user's endpoint is not updated
	if _, err := queries.UpdateEndpoint(ctx, db.UpdateEndpointParams{
		ID: "ep", UserID: "user-2", Muted: sql.NullInt64{Int64: 0, Valid: true},
	}); err != sql.ErrNoRows {
		t.Errorf("update as another user = %v, want sql.ErrNoRows", err)
	}
}
