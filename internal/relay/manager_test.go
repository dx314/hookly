package relay

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	hooklyv1 "hooks.dx314.com/internal/api/hookly/v1"
	"hooks.dx314.com/internal/proxy"
)

// A second hub can't take an endpoint a live hub relays; the same hub
// reconnecting can, and so can anyone once the holder goes stale.
func TestTryAddConnectionEndpointConflict(t *testing.T) {
	mgr := NewConnectionManager()
	first, conflict := mgr.TryAddConnection("host-schoolboy", []string{"ep"}, nil, nil, time.Minute)
	if conflict != nil {
		t.Fatalf("first connection conflicted: %+v", conflict)
	}

	if _, conflict := mgr.TryAddConnection("host-homeboy", []string{"other", "ep"}, nil, nil, time.Minute); conflict == nil ||
		conflict.EndpointID != "ep" || conflict.HubID != "host-schoolboy" {
		t.Fatalf("conflict = %+v, want ep held by host-schoolboy", conflict)
	}
	if mgr.GetHubForEndpoint("ep") != first || mgr.GetHubForEndpoint("other") != nil {
		t.Fatal("rejected connection changed routing")
	}

	// Different endpoints coexist
	if _, conflict := mgr.TryAddConnection("host-homeboy", []string{"other"}, nil, nil, time.Minute); conflict != nil {
		t.Fatalf("disjoint endpoints conflicted: %+v", conflict)
	}

	// Same hub reconnecting replaces itself
	again, conflict := mgr.TryAddConnection("host-schoolboy", []string{"ep"}, nil, nil, time.Minute)
	if conflict != nil || mgr.GetHubForEndpoint("ep") != again {
		t.Fatalf("reconnect: conflict %+v", conflict)
	}

	// A stale holder can be taken over, and its later removal leaves the new route
	mgr.mu.Lock()
	again.lastHeartbeat = time.Now().Add(-2 * time.Minute)
	mgr.mu.Unlock()
	taker, conflict := mgr.TryAddConnection("host-new", []string{"ep"}, nil, nil, time.Minute)
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

// A proxied request goes down the hub's proxy channel and returns when the
// matching response arrives; the stream dropping fails every waiter.
func TestHubConnectionProxy(t *testing.T) {
	mgr := NewConnectionManager()
	conn, _ := mgr.TryAddConnection("infocube", []string{"ep"}, []string{CapabilityFanout, CapabilityProxy}, []string{"homeboy"}, time.Minute)

	if mgr.FindProxy("infocube", "schoolboy") != nil {
		t.Error("FindProxy returned a hub for a name it does not serve")
	}
	if mgr.FindProxy("other", "homeboy") != nil {
		t.Error("FindProxy returned a hub for an unknown hub ID")
	}
	if hub := mgr.FindProxy("infocube", "homeboy"); hub == nil {
		t.Fatal("FindProxy found nothing for the advertised name")
	}

	// Hub without the capability is never sent a request, even if it lists names
	mgr.TryAddConnection("old", []string{"ep2"}, []string{CapabilityFanout}, []string{"homeboy"}, time.Minute)
	if mgr.FindProxy("old", "homeboy") != nil {
		t.Error("FindProxy returned a hub without the proxy capability")
	}

	// Round trip
	type result struct {
		resp *hooklyv1.HttpResponse
		err  error
	}
	results := make(chan result, 1)
	go func() {
		resp, err := conn.Proxy(context.Background(), &hooklyv1.HttpRequest{RequestId: "r1", Proxy: "homeboy", Method: "GET", Path: "/app/"})
		results <- result{resp, err}
	}()
	select {
	case msg := <-conn.ProxyCh():
		if msg.GetHttpRequest().GetRequestId() != "r1" {
			t.Fatalf("stream got %v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("request never reached the stream")
	}
	conn.ResolveProxy(&hooklyv1.HttpResponse{RequestId: "unknown", Status: 500}) // ignored
	conn.ResolveProxy(&hooklyv1.HttpResponse{RequestId: "r1", Status: 200})
	select {
	case r := <-results:
		if r.err != nil || r.resp.Status != 200 {
			t.Fatalf("Proxy returned %v, %v", r.resp, r.err)
		}
	case <-time.After(time.Second):
		t.Fatal("Proxy did not return after the response")
	}
	if conn.PendingProxy() != 0 {
		t.Errorf("%d requests still pending", conn.PendingProxy())
	}

	// Caller timeout cleans up
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := conn.Proxy(ctx, &hooklyv1.HttpRequest{RequestId: "r2"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("timed-out Proxy returned %v", err)
	}
	<-conn.ProxyCh()
	if conn.PendingProxy() != 0 {
		t.Errorf("%d requests pending after timeout", conn.PendingProxy())
	}

	// Disconnect fails whoever is waiting
	go func() {
		_, err := conn.Proxy(context.Background(), &hooklyv1.HttpRequest{RequestId: "r3"})
		results <- result{nil, err}
	}()
	<-conn.ProxyCh()
	mgr.RemoveConnection(conn)
	select {
	case r := <-results:
		if !errors.Is(r.err, proxy.ErrDisconnected) {
			t.Errorf("after disconnect Proxy returned %v, want ErrDisconnected", r.err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiter not failed on disconnect")
	}
	if _, err := conn.Proxy(context.Background(), &hooklyv1.HttpRequest{RequestId: "r4"}); !errors.Is(err, proxy.ErrDisconnected) {
		t.Errorf("Proxy on a closed connection returned %v", err)
	}

	// The same hub reconnecting also fails the old connection's waiters
	first, _ := mgr.TryAddConnection("infocube", nil, []string{CapabilityProxy}, []string{"homeboy"}, time.Minute)
	go func() {
		_, err := first.Proxy(context.Background(), &hooklyv1.HttpRequest{RequestId: "r5"})
		results <- result{nil, err}
	}()
	<-first.ProxyCh()
	mgr.TryAddConnection("infocube", nil, []string{CapabilityProxy}, []string{"homeboy"}, time.Minute)
	select {
	case r := <-results:
		if !errors.Is(r.err, proxy.ErrDisconnected) {
			t.Errorf("after replacement Proxy returned %v, want ErrDisconnected", r.err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiter not failed on replacement")
	}
}

// Beyond maxPendingProxy in-flight requests a hub answers busy instead of
// queueing without bound.
func TestHubConnectionProxyBusy(t *testing.T) {
	mgr := NewConnectionManager()
	conn, _ := mgr.TryAddConnection("infocube", nil, []string{CapabilityProxy}, []string{"homeboy"}, time.Minute)
	defer mgr.RemoveConnection(conn)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < maxPendingProxy; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			conn.Proxy(ctx, &hooklyv1.HttpRequest{RequestId: fmt.Sprint(i)})
		}(i)
	}
	deadline := time.Now().Add(2 * time.Second)
	for conn.PendingProxy() < maxPendingProxy && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := conn.Proxy(ctx, &hooklyv1.HttpRequest{RequestId: "extra"}); !errors.Is(err, proxy.ErrBusy) {
		t.Errorf("Proxy over the cap returned %v, want ErrBusy", err)
	}
	cancel()
	wg.Wait()
}
