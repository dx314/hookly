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
	if mgr.IsAnyConnected() {
		t.Error("expected no connections initially")
	}

	// Add connection with endpoints
	conn := mgr.AddConnection("hub-1", []string{"ep-1", "ep-2"})
	if !mgr.IsAnyConnected() {
		t.Error("expected connected after AddConnection")
	}

	// Check endpoint routing
	if mgr.GetHubForEndpoint("ep-1") != conn {
		t.Error("expected ep-1 to route to hub-1")
	}
	if mgr.GetHubForEndpoint("ep-2") != conn {
		t.Error("expected ep-2 to route to hub-1")
	}
	if mgr.GetHubForEndpoint("ep-unknown") != nil {
		t.Error("expected unknown endpoint to return nil")
	}

	// Heartbeat
	mgr.UpdateHeartbeat("hub-1")
	if mgr.IsStale("hub-1", 1*time.Second) {
		t.Error("should not be stale right after heartbeat")
	}

	// Remove connection
	mgr.RemoveConnection("hub-1")
	if mgr.IsAnyConnected() {
		t.Error("expected not connected after RemoveConnection")
	}
	if mgr.GetHubForEndpoint("ep-1") != nil {
		t.Error("expected ep-1 to return nil after hub removed")
	}
}

func TestMultipleHubs(t *testing.T) {
	mgr := NewConnectionManager()

	// Add two hubs with different endpoints
	conn1 := mgr.AddConnection("hub-1", []string{"ep-1", "ep-2"})
	conn2 := mgr.AddConnection("hub-2", []string{"ep-3", "ep-4"})

	// Check routing
	if mgr.GetHubForEndpoint("ep-1") != conn1 {
		t.Error("expected ep-1 to route to hub-1")
	}
	if mgr.GetHubForEndpoint("ep-3") != conn2 {
		t.Error("expected ep-3 to route to hub-2")
	}

	// Remove one hub
	mgr.RemoveConnection("hub-1")

	// Hub 2 should still work
	if mgr.GetHubForEndpoint("ep-3") != conn2 {
		t.Error("expected ep-3 to still route to hub-2")
	}
	if mgr.GetHubForEndpoint("ep-1") != nil {
		t.Error("expected ep-1 to return nil after hub-1 removed")
	}
}
