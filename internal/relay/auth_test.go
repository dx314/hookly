package relay

import (
	"testing"
	"time"
)

func TestGenerateAndValidateHMAC(t *testing.T) {
	hubID := "home-hub-1"
	secret := "test-secret-key"
	timestamp := time.Now().Unix()

	// Generate HMAC
	hmac := GenerateHMAC(hubID, timestamp, secret)

	// Should validate with correct params
	if !ValidateHMAC(hubID, timestamp, hmac, secret) {
		t.Error("expected valid HMAC to pass")
	}

	// Should fail with wrong secret
	if ValidateHMAC(hubID, timestamp, hmac, "wrong-secret") {
		t.Error("expected wrong secret to fail")
	}

	// Should fail with wrong hubID
	if ValidateHMAC("other-hub", timestamp, hmac, secret) {
		t.Error("expected wrong hubID to fail")
	}

	// Should fail with expired timestamp (>5 min old)
	oldTimestamp := time.Now().Unix() - 400 // 6+ minutes ago
	oldHMAC := GenerateHMAC(hubID, oldTimestamp, secret)
	if ValidateHMAC(hubID, oldTimestamp, oldHMAC, secret) {
		t.Error("expected expired timestamp to fail")
	}

	// Should fail with future timestamp (>5 min in future)
	futureTimestamp := time.Now().Unix() + 400 // 6+ minutes in future
	futureHMAC := GenerateHMAC(hubID, futureTimestamp, secret)
	if ValidateHMAC(hubID, futureTimestamp, futureHMAC, secret) {
		t.Error("expected future timestamp to fail")
	}
}

func TestConnectionManager(t *testing.T) {
	mgr := NewConnectionManager()

	// Initially not connected
	if mgr.IsConnected() {
		t.Error("expected not connected initially")
	}

	// Connect
	mgr.SetConnected("hub-1")
	if !mgr.IsConnected() {
		t.Error("expected connected after SetConnected")
	}

	// Heartbeat
	mgr.UpdateHeartbeat()
	if mgr.IsStale(1 * time.Second) {
		t.Error("should not be stale right after heartbeat")
	}

	// Disconnect
	mgr.SetDisconnected()
	if mgr.IsConnected() {
		t.Error("expected not connected after SetDisconnected")
	}
}
