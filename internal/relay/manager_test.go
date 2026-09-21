package relay

import (
	"errors"
	"testing"
	"time"
)

// A second hub can't take an endpoint a live hub relays; the same hub
// reconnecting can, and so can anyone once the holder goes stale.
func TestTryAddConnectionEndpointConflict(t *testing.T) {
	mgr := NewConnectionManager()
	first, conflict := mgr.TryAddConnection("host-schoolboy", []string{"ep"}, nil, time.Minute)
	if conflict != nil {
		t.Fatalf("first connection conflicted: %+v", conflict)
	}

	if _, conflict := mgr.TryAddConnection("host-homeboy", []string{"other", "ep"}, nil, time.Minute); conflict == nil ||
		conflict.EndpointID != "ep" || conflict.HubID != "host-schoolboy" {
		t.Fatalf("conflict = %+v, want ep held by host-schoolboy", conflict)
	}
	if mgr.GetHubForEndpoint("ep") != first || mgr.GetHubForEndpoint("other") != nil {
		t.Fatal("rejected connection changed routing")
	}

	// Different endpoints coexist
	if _, conflict := mgr.TryAddConnection("host-homeboy", []string{"other"}, nil, time.Minute); conflict != nil {
		t.Fatalf("disjoint endpoints conflicted: %+v", conflict)
	}

	// Same hub reconnecting replaces itself
	again, conflict := mgr.TryAddConnection("host-schoolboy", []string{"ep"}, nil, time.Minute)
	if conflict != nil || mgr.GetHubForEndpoint("ep") != again {
		t.Fatalf("reconnect: conflict %+v", conflict)
	}

	// A stale holder can be taken over, and its later removal leaves the new route
	mgr.mu.Lock()
	again.lastHeartbeat = time.Now().Add(-2 * time.Minute)
	mgr.mu.Unlock()
	taker, conflict := mgr.TryAddConnection("host-new", []string{"ep"}, nil, time.Minute)
	if conflict != nil {
		t.Fatalf("stale holder blocked takeover: %+v", conflict)
	}
	mgr.RemoveConnection(again)
	if mgr.GetHubForEndpoint("ep") != taker {
		t.Error("removing the stale hub dropped the new hub's route")
	}
}

func TestParseConnectErrorEndpointInUse(t *testing.T) {
	err := parseConnectError("ENDPOINT_IN_USE: endpoint 'ep' is already relayed by hub 'h'")
	if !errors.Is(err, ErrEndpointInUse) {
		t.Fatalf("err = %v, want ErrEndpointInUse", err)
	}
	if isPermanentError(err) {
		t.Error("ENDPOINT_IN_USE must be retried")
	}
}
