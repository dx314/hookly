package webhook

import (
	"testing"
	"time"
)

func TestStripeVerifier(t *testing.T) {
	v := &StripeVerifier{}
	secret := "whsec_test_secret"
	payload := []byte(`{"type":"payment_intent.succeeded"}`)
	timestamp := time.Now().Unix()

	// Generate valid signature
	sig := ComputeStripeSignature(payload, secret, timestamp)
	headers := map[string]string{"Stripe-Signature": sig}

	if !v.Verify(payload, headers, secret) {
		t.Error("expected valid signature to pass")
	}

	// Test with wrong secret
	if v.Verify(payload, headers, "wrong_secret") {
		t.Error("expected wrong secret to fail")
	}

	// Test with old timestamp (>5 min)
	oldTimestamp := time.Now().Unix() - 400
	oldSig := ComputeStripeSignature(payload, secret, oldTimestamp)
	oldHeaders := map[string]string{"Stripe-Signature": oldSig}
	if v.Verify(payload, oldHeaders, secret) {
		t.Error("expected old timestamp to fail")
	}

	// Test with missing signature
	if v.Verify(payload, map[string]string{}, secret) {
		t.Error("expected missing signature to fail")
	}
}

func TestGitHubVerifier(t *testing.T) {
	v := &GitHubVerifier{}
	secret := "github_secret"
	payload := []byte(`{"action":"opened"}`)

	// Generate valid signature
	sig := ComputeGitHubSignature(payload, secret)
	headers := map[string]string{"X-Hub-Signature-256": sig}

	if !v.Verify(payload, headers, secret) {
		t.Error("expected valid signature to pass")
	}

	// Test with wrong secret
	if v.Verify(payload, headers, "wrong_secret") {
		t.Error("expected wrong secret to fail")
	}

	// Test with tampered payload
	if v.Verify([]byte(`{"action":"closed"}`), headers, secret) {
		t.Error("expected tampered payload to fail")
	}

	// Test case-insensitive header
	lowercaseHeaders := map[string]string{"x-hub-signature-256": sig}
	if !v.Verify(payload, lowercaseHeaders, secret) {
		t.Error("expected case-insensitive header to work")
	}
}

func TestTelegramVerifier(t *testing.T) {
	v := &TelegramVerifier{}
	secret := "telegram_token"
	payload := []byte(`{"update_id":123}`)
	headers := map[string]string{"X-Telegram-Bot-Api-Secret-Token": secret}

	if !v.Verify(payload, headers, secret) {
		t.Error("expected valid token to pass")
	}

	// Test with wrong token
	wrongHeaders := map[string]string{"X-Telegram-Bot-Api-Secret-Token": "wrong"}
	if v.Verify(payload, wrongHeaders, secret) {
		t.Error("expected wrong token to fail")
	}
}

func TestGenericVerifier(t *testing.T) {
	v := &GenericVerifier{}
	secret := "generic_secret"
	payload := []byte(`{"event":"test"}`)

	// Generate valid signature (same format as GitHub)
	sig := ComputeGitHubSignature(payload, secret)
	headers := map[string]string{"X-Webhook-Signature": sig}

	if !v.Verify(payload, headers, secret) {
		t.Error("expected valid signature to pass")
	}

	// Test with missing signature
	if v.Verify(payload, map[string]string{}, secret) {
		t.Error("expected missing signature to fail")
	}
}

func TestNewVerifier(t *testing.T) {
	tests := []struct {
		providerType string
		expected     string
	}{
		{"stripe", "*webhook.StripeVerifier"},
		{"github", "*webhook.GitHubVerifier"},
		{"telegram", "*webhook.TelegramVerifier"},
		{"generic", "*webhook.GenericVerifier"},
		{"unknown", "*webhook.GenericVerifier"}, // defaults to generic
	}

	for _, tt := range tests {
		v := NewVerifier(tt.providerType)
		switch v.(type) {
		case *StripeVerifier:
			if tt.providerType != "stripe" {
				t.Errorf("expected %s verifier for %s", tt.expected, tt.providerType)
			}
		case *GitHubVerifier:
			if tt.providerType != "github" {
				t.Errorf("expected %s verifier for %s", tt.expected, tt.providerType)
			}
		case *TelegramVerifier:
			if tt.providerType != "telegram" {
				t.Errorf("expected %s verifier for %s", tt.expected, tt.providerType)
			}
		case *GenericVerifier:
			if tt.providerType != "generic" && tt.providerType != "unknown" {
				t.Errorf("expected %s verifier for %s", tt.expected, tt.providerType)
			}
		}
	}
}
