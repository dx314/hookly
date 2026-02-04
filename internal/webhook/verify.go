// Package webhook handles webhook ingestion and signature verification.
package webhook

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
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
// For "custom" provider type, use NewCustomVerifier instead.
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
	case "custom":
		// Custom requires config; return nil to signal caller should use NewCustomVerifier
		return nil
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

// VerificationMethod defines the type of signature verification.
type VerificationMethod string

const (
	// MethodStatic compares header value directly against the secret.
	MethodStatic VerificationMethod = "static"
	// MethodHMACSHA256 computes HMAC-SHA256 of payload.
	MethodHMACSHA256 VerificationMethod = "hmac_sha256"
	// MethodHMACSHA1 computes HMAC-SHA1 of payload.
	MethodHMACSHA1 VerificationMethod = "hmac_sha1"
	// MethodTimestampedHMAC uses timestamp + payload for HMAC (like Stripe).
	MethodTimestampedHMAC VerificationMethod = "timestamped_hmac"
)

// VerificationConfig defines custom verification settings.
type VerificationConfig struct {
	// Method is the verification method to use.
	Method VerificationMethod `json:"method"`
	// SignatureHeader is the header containing the signature.
	SignatureHeader string `json:"signature_header"`
	// SignaturePrefix is an optional prefix to strip (e.g., "sha256=").
	SignaturePrefix string `json:"signature_prefix,omitempty"`
	// TimestampHeader is the header containing the timestamp (for timestamped_hmac).
	TimestampHeader string `json:"timestamp_header,omitempty"`
	// TimestampTolerance is max age in seconds (default 300 for timestamped_hmac).
	TimestampTolerance int64 `json:"timestamp_tolerance,omitempty"`
}

// ParseVerificationConfig parses JSON config into VerificationConfig.
func ParseVerificationConfig(data []byte) (*VerificationConfig, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty verification config")
	}
	var cfg VerificationConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid verification config: %w", err)
	}
	if cfg.SignatureHeader == "" {
		return nil, fmt.Errorf("signature_header is required")
	}
	if cfg.Method == "" {
		return nil, fmt.Errorf("method is required")
	}
	switch cfg.Method {
	case MethodStatic, MethodHMACSHA256, MethodHMACSHA1, MethodTimestampedHMAC:
		// valid
	default:
		return nil, fmt.Errorf("invalid method: %s", cfg.Method)
	}
	if cfg.Method == MethodTimestampedHMAC && cfg.TimestampHeader == "" {
		return nil, fmt.Errorf("timestamp_header is required for timestamped_hmac method")
	}
	return &cfg, nil
}

// CustomVerifier verifies webhooks using custom configuration.
type CustomVerifier struct {
	Config *VerificationConfig
}

// NewCustomVerifier creates a verifier with the given config.
func NewCustomVerifier(cfg *VerificationConfig) *CustomVerifier {
	return &CustomVerifier{Config: cfg}
}

func (v *CustomVerifier) Verify(payload []byte, headers map[string]string, secret string) bool {
	if v.Config == nil {
		return false
	}

	sig := getHeader(headers, v.Config.SignatureHeader)
	if sig == "" {
		return false
	}

	// Strip prefix if configured
	if v.Config.SignaturePrefix != "" {
		if !strings.HasPrefix(sig, v.Config.SignaturePrefix) {
			return false
		}
		sig = strings.TrimPrefix(sig, v.Config.SignaturePrefix)
	}

	switch v.Config.Method {
	case MethodStatic:
		return subtle.ConstantTimeCompare([]byte(sig), []byte(secret)) == 1

	case MethodHMACSHA256:
		sigBytes, err := hex.DecodeString(sig)
		if err != nil {
			return false
		}
		expected := computeHMACSHA256(payload, []byte(secret))
		return subtle.ConstantTimeCompare(expected, sigBytes) == 1

	case MethodHMACSHA1:
		sigBytes, err := hex.DecodeString(sig)
		if err != nil {
			return false
		}
		expected := computeHMACSHA1(payload, []byte(secret))
		return subtle.ConstantTimeCompare(expected, sigBytes) == 1

	case MethodTimestampedHMAC:
		timestamp := getHeader(headers, v.Config.TimestampHeader)
		if timestamp == "" {
			return false
		}
		ts, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			return false
		}
		tolerance := v.Config.TimestampTolerance
		if tolerance == 0 {
			tolerance = 300 // default 5 minutes
		}
		if time.Now().Unix()-ts > tolerance {
			return false
		}
		signedPayload := timestamp + "." + string(payload)
		sigBytes, err := hex.DecodeString(sig)
		if err != nil {
			return false
		}
		expected := computeHMACSHA256([]byte(signedPayload), []byte(secret))
		return subtle.ConstantTimeCompare(expected, sigBytes) == 1

	default:
		return false
	}
}

// computeHMACSHA1 computes HMAC-SHA1.
func computeHMACSHA1(message, key []byte) []byte {
	mac := hmac.New(sha1.New, key)
	mac.Write(message)
	return mac.Sum(nil)
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
