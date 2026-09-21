package db

import "database/sql"

// Delivery and webhook statuses.
const (
	StatusPending    = "pending"
	StatusDelivered  = "delivered"
	StatusFailed     = "failed"
	StatusDeadLetter = "dead_letter"
)

// DeliveryState is the part of a delivery that feeds a webhook's derived status.
type DeliveryState struct {
	Status        string
	Attempts      int64
	LastAttemptAt sql.NullString
	DeliveredAt   sql.NullString
	ErrorMessage  sql.NullString
	// Enabled is whether the delivery's destination is currently enabled.
	Enabled bool
}

// WebhookRollup is the webhook-level view derived from its deliveries.
type WebhookRollup struct {
	Status        string
	Attempts      int64
	LastAttemptAt sql.NullString
	DeliveredAt   sql.NullString
	ErrorMessage  sql.NullString
}

// statusSeverity orders statuses so the rollup surfaces the worst one.
var statusSeverity = map[string]int{
	StatusDelivered:  0,
	StatusPending:    1,
	StatusFailed:     2,
	StatusDeadLetter: 3,
}

// Rollup derives a webhook's overall status from its per-destination deliveries.
//
//   - delivered: every delivery is delivered
//   - dead_letter / failed: any delivery is (dead_letter wins over failed)
//   - pending: otherwise
//
// Only deliveries to enabled destinations count; if every destination is
// disabled all deliveries count. ok is false when there are no deliveries, in
// which case the webhook's stored status should be left alone.
func Rollup(deliveries []DeliveryState) (rollup WebhookRollup, ok bool) {
	considered := make([]DeliveryState, 0, len(deliveries))
	for _, d := range deliveries {
		if d.Enabled {
			considered = append(considered, d)
		}
	}
	if len(considered) == 0 {
		considered = deliveries
	}
	if len(considered) == 0 {
		return WebhookRollup{}, false
	}

	rollup.Status = StatusDelivered
	for _, d := range considered {
		if statusSeverity[d.Status] > statusSeverity[rollup.Status] {
			rollup.Status = d.Status
		}
		rollup.Attempts = max(rollup.Attempts, d.Attempts)
		rollup.LastAttemptAt = laterTime(rollup.LastAttemptAt, d.LastAttemptAt)
	}

	// delivered_at and error_message come from the deliveries that decided the status.
	var errorAt sql.NullString
	for _, d := range considered {
		if d.Status != rollup.Status {
			continue
		}
		if rollup.Status == StatusDelivered {
			rollup.DeliveredAt = laterTime(rollup.DeliveredAt, d.DeliveredAt)
			continue
		}
		if d.ErrorMessage.Valid && (!rollup.ErrorMessage.Valid || laterTime(errorAt, d.LastAttemptAt) != errorAt) {
			rollup.ErrorMessage = d.ErrorMessage
			errorAt = d.LastAttemptAt
		}
	}

	return rollup, true
}

// laterTime returns the later of two SQLite datetime strings (which sort lexically).
func laterTime(a, b sql.NullString) sql.NullString {
	if !a.Valid {
		return b
	}
	if !b.Valid || a.String >= b.String {
		return a
	}
	return b
}
