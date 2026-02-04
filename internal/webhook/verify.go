// Package webhook handles webhook ingestion and signature verification.
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Verifier verifies webhook signatures.
type Verifier interface {
	// Verify checks if the signature is valid for the given payload and headers.
	// Returns true if valid, false if invalid.
	Verify(payload []byte, headers map[string]string, secret string) bool
}

// NewVerifier creates a verifier for the given provider type.
func NewVerifier(providerType string) Verifier {
	switch providerType {
	case "stripe":
		return &StripeVerifier{}
	case "github":
		return &GitHubVerifier{}
	case "telegram":
		return &TelegramVerifier{}
	case "generic":
		return &GenericVerifier{}
	default:
		return &GenericVerifier{}
	}
}

// StripeVerifier verifies Stripe webhook signatures.
// Format: Stripe-Signature: t=1492774577,v1=5257a869...
type StripeVerifier struct{}

func (v *StripeVerifier) Verify(payload []byte, headers map[string]string, secret string) bool {
	sig := getHeader(headers, "Stripe-Signature")
	if sig == "" {
		return false
	}

	// Parse signature header
	var timestamp string
	var signatures []string

	parts := strings.Split(sig, ",")
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])

		switch key {
		case "t":
			timestamp = val
		case "v1":
			signatures = append(signatures, val)
		}
	}

	if timestamp == "" || len(signatures) == 0 {
		return false
	}

	// Validate timestamp (within 5 minutes)
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix()-ts > 300 { // 5 minutes
		return false
	}

	// Compute expected signature
	signedPayload := timestamp + "." + string(payload)
	expected := computeHMACSHA256([]byte(signedPayload), []byte(secret))

	// Check against all v1 signatures (Stripe may include multiple)
	for _, sig := range signatures {
		sigBytes, err := hex.DecodeString(sig)
		if err != nil {
			continue
		}
		if subtle.ConstantTimeCompare(expected, sigBytes) == 1 {
			return true
		}
	}

	return false
}

// GitHubVerifier verifies GitHub webhook signatures.
// Format: X-Hub-Signature-256: sha256=d57c68ca...
type GitHubVerifier struct{}

func (v *GitHubVerifier) Verify(payload []byte, headers map[string]string, secret string) bool {
	sig := getHeader(headers, "X-Hub-Signature-256")
	if sig == "" {
		return false
	}

	// Parse signature (remove "sha256=" prefix)
	if !strings.HasPrefix(sig, "sha256=") {
		return false
	}
	sigHex := strings.TrimPrefix(sig, "sha256=")

	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}

	expected := computeHMACSHA256(payload, []byte(secret))
	return subtle.ConstantTimeCompare(expected, sigBytes) == 1
}

// TelegramVerifier verifies Telegram webhook secret tokens.
// Format: X-Telegram-Bot-Api-Secret-Token: <secret>
type TelegramVerifier struct{}

func (v *TelegramVerifier) Verify(payload []byte, headers map[string]string, secret string) bool {
	token := getHeader(headers, "X-Telegram-Bot-Api-Secret-Token")
	if token == "" {
		return false
	}

	// Simple constant-time string comparison
	return subtle.ConstantTimeCompare([]byte(token), []byte(secret)) == 1
}

// GenericVerifier verifies generic webhook signatures.
// Format: X-Webhook-Signature: sha256=...
type GenericVerifier struct{}

func (v *GenericVerifier) Verify(payload []byte, headers map[string]string, secret string) bool {
	sig := getHeader(headers, "X-Webhook-Signature")
	if sig == "" {
		// No signature header - can't verify
		return false
	}

	// Parse signature (remove "sha256=" prefix)
	if !strings.HasPrefix(sig, "sha256=") {
		return false
	}
	sigHex := strings.TrimPrefix(sig, "sha256=")

	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}

	expected := computeHMACSHA256(payload, []byte(secret))
	return subtle.ConstantTimeCompare(expected, sigBytes) == 1
}

// computeHMACSHA256 computes HMAC-SHA256.
func computeHMACSHA256(message, key []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(message)
	return mac.Sum(nil)
}

// getHeader gets a header value case-insensitively.
func getHeader(headers map[string]string, name string) string {
	// Try exact match first
	if val, ok := headers[name]; ok {
		return val
	}
	// Try case-insensitive match
	nameLower := strings.ToLower(name)
	for k, v := range headers {
		if strings.ToLower(k) == nameLower {
			return v
		}
	}
	return ""
}

// ComputeStripeSignature generates a Stripe signature for testing.
func ComputeStripeSignature(payload []byte, secret string, timestamp int64) string {
	signedPayload := fmt.Sprintf("%d.%s", timestamp, string(payload))
	sig := computeHMACSHA256([]byte(signedPayload), []byte(secret))
	return fmt.Sprintf("t=%d,v1=%s", timestamp, hex.EncodeToString(sig))
}

// ComputeGitHubSignature generates a GitHub signature for testing.
func ComputeGitHubSignature(payload []byte, secret string) string {
	sig := computeHMACSHA256(payload, []byte(secret))
	return "sha256=" + hex.EncodeToString(sig)
}
