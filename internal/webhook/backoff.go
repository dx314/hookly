package webhook

import "time"

// MaxRetryDelay is the maximum delay between retries (1 hour).
const MaxRetryDelay = time.Hour

// NextRetryDelay calculates the next retry delay using exponential backoff.
// Returns: 1s, 2s, 4s, 8s, 16s, 32s, 64s, 128s, 256s, 512s, 1024s, 2048s, max 1 hour.
func NextRetryDelay(attempts int) time.Duration {
	attempts = max(0, min(attempts, 12))
	delay := time.Second * time.Duration(1<<attempts)
	return min(delay, MaxRetryDelay)
}

// NextRetryTime calculates when a webhook should next be retried.
func NextRetryTime(lastAttempt time.Time, attempts int) time.Time {
	return lastAttempt.Add(NextRetryDelay(attempts))
}

// ShouldRetry returns true if enough time has passed since the last attempt.
func ShouldRetry(lastAttempt time.Time, attempts int) bool {
	return time.Now().After(NextRetryTime(lastAttempt, attempts))
}
