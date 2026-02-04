// Package relay handles the gRPC streaming connection between edge and home-hub.
package relay

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"time"
)

const (
	// maxTimestampDrift is the maximum allowed time difference for auth.
	maxTimestampDrift = 5 * time.Minute
)

// GenerateHMAC generates an HMAC for authentication.
// Format: HMAC-SHA256(hubID + ":" + timestamp, secret)
func GenerateHMAC(hubID string, timestamp int64, secret string) string {
	message := fmt.Sprintf("%s:%d", hubID, timestamp)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// ValidateHMAC validates an HMAC from a connect request.
func ValidateHMAC(hubID string, timestamp int64, signature, secret string) bool {
	// Check timestamp is within acceptable range
	now := time.Now().Unix()
	drift := now - timestamp
	if drift < 0 {
		drift = -drift
	}
	if drift > int64(maxTimestampDrift.Seconds()) {
		return false
	}

	// Compute expected HMAC
	expected := GenerateHMAC(hubID, timestamp, secret)

	// Constant-time comparison
	return subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) == 1
}
