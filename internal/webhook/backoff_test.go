package webhook

import (
	"testing"
	"time"
)

func TestNextRetryDelay(t *testing.T) {
	tests := []struct {
		attempts int
		want     time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 32 * time.Second},
		{6, 64 * time.Second},
		{7, 128 * time.Second},
		{8, 256 * time.Second},
		{9, 512 * time.Second},
		{10, 1024 * time.Second},
		{11, 2048 * time.Second},
		{12, time.Hour}, // Capped at max
		{13, time.Hour}, // Still capped
		{100, time.Hour}, // High value capped
	}

	for _, tt := range tests {
		got := NextRetryDelay(tt.attempts)
		if got != tt.want {
			t.Errorf("NextRetryDelay(%d) = %v, want %v", tt.attempts, got, tt.want)
		}
	}
}

func TestNextRetryDelay_Negative(t *testing.T) {
	// Negative attempts should be treated as 0
	got := NextRetryDelay(-1)
	want := 1 * time.Second
	if got != want {
		t.Errorf("NextRetryDelay(-1) = %v, want %v", got, want)
	}
}

func TestShouldRetry(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		lastAttempt time.Time
		attempts    int
		want        bool
	}{
		{
			name:        "first attempt - should retry immediately",
			lastAttempt: now.Add(-2 * time.Second),
			attempts:    0,
			want:        true, // 1s backoff has passed
		},
		{
			name:        "first attempt - too soon",
			lastAttempt: now.Add(-500 * time.Millisecond),
			attempts:    0,
			want:        false, // 1s backoff hasn't passed
		},
		{
			name:        "second attempt - should retry",
			lastAttempt: now.Add(-3 * time.Second),
			attempts:    1,
			want:        true, // 2s backoff has passed
		},
		{
			name:        "second attempt - too soon",
			lastAttempt: now.Add(-1 * time.Second),
			attempts:    1,
			want:        false, // 2s backoff hasn't passed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldRetry(tt.lastAttempt, tt.attempts)
			if got != tt.want {
				t.Errorf("ShouldRetry() = %v, want %v", got, tt.want)
			}
		})
	}
}
