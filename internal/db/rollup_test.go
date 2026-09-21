package db_test

import (
	"database/sql"
	"testing"

	"hooks.dx314.com/internal/db"
)

func ns(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

func TestRollup(t *testing.T) {
	delivered := db.DeliveryState{Status: db.StatusDelivered, Attempts: 1, LastAttemptAt: ns("2026-09-21 10:00:01"), DeliveredAt: ns("2026-09-21 10:00:01"), Enabled: true}
	deliveredLater := db.DeliveryState{Status: db.StatusDelivered, Attempts: 3, LastAttemptAt: ns("2026-09-21 10:00:09"), DeliveredAt: ns("2026-09-21 10:00:09"), Enabled: true}
	fresh := db.DeliveryState{Status: db.StatusPending, Enabled: true}
	retrying := db.DeliveryState{Status: db.StatusPending, Attempts: 2, LastAttemptAt: ns("2026-09-21 10:00:05"), ErrorMessage: ns("HTTP 500"), Enabled: true}
	failed := db.DeliveryState{Status: db.StatusFailed, Attempts: 1, LastAttemptAt: ns("2026-09-21 10:00:02"), ErrorMessage: ns("HTTP 404"), Enabled: true}
	dead := db.DeliveryState{Status: db.StatusDeadLetter, Attempts: 180, LastAttemptAt: ns("2026-09-28 10:00:00"), ErrorMessage: ns("network error"), Enabled: true}

	disabled := func(d db.DeliveryState) db.DeliveryState { d.Enabled = false; return d }

	tests := []struct {
		name        string
		deliveries  []db.DeliveryState
		wantOK      bool
		wantStatus  string
		wantAttempt int64
		wantError   string
		wantDelAt   string
	}{
		{name: "no deliveries leaves webhook alone", deliveries: nil, wantOK: false},
		{name: "single pending", deliveries: []db.DeliveryState{fresh}, wantOK: true, wantStatus: db.StatusPending},
		{name: "single delivered", deliveries: []db.DeliveryState{delivered}, wantOK: true, wantStatus: db.StatusDelivered, wantAttempt: 1, wantDelAt: "2026-09-21 10:00:01"},
		{name: "all delivered uses latest delivered_at", deliveries: []db.DeliveryState{delivered, deliveredLater}, wantOK: true, wantStatus: db.StatusDelivered, wantAttempt: 3, wantDelAt: "2026-09-21 10:00:09"},
		{name: "one delivered one retrying is pending", deliveries: []db.DeliveryState{delivered, retrying}, wantOK: true, wantStatus: db.StatusPending, wantAttempt: 2, wantError: "HTTP 500"},
		{name: "any failed is failed", deliveries: []db.DeliveryState{delivered, failed}, wantOK: true, wantStatus: db.StatusFailed, wantAttempt: 1, wantError: "HTTP 404"},
		{name: "failed wins over pending", deliveries: []db.DeliveryState{retrying, failed}, wantOK: true, wantStatus: db.StatusFailed, wantAttempt: 2, wantError: "HTTP 404"},
		{name: "dead letter wins over failed", deliveries: []db.DeliveryState{failed, dead, delivered}, wantOK: true, wantStatus: db.StatusDeadLetter, wantAttempt: 180, wantError: "network error"},
		{name: "disabled destination is ignored", deliveries: []db.DeliveryState{delivered, disabled(retrying)}, wantOK: true, wantStatus: db.StatusDelivered, wantAttempt: 1, wantDelAt: "2026-09-21 10:00:01"},
		{name: "all disabled falls back to all deliveries", deliveries: []db.DeliveryState{disabled(delivered), disabled(failed)}, wantOK: true, wantStatus: db.StatusFailed, wantAttempt: 1, wantError: "HTTP 404"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := db.Rollup(tt.deliveries)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if got.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", got.Status, tt.wantStatus)
			}
			if got.Attempts != tt.wantAttempt {
				t.Errorf("attempts = %d, want %d", got.Attempts, tt.wantAttempt)
			}
			if got.ErrorMessage.String != tt.wantError {
				t.Errorf("error = %q, want %q", got.ErrorMessage.String, tt.wantError)
			}
			if got.DeliveredAt.String != tt.wantDelAt {
				t.Errorf("delivered_at = %q, want %q", got.DeliveredAt.String, tt.wantDelAt)
			}
		})
	}
}
