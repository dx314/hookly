package relay

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	hooklyv1 "hooks.dx314.com/internal/api/hookly/v1"
	"hooks.dx314.com/internal/proxy"
)

// CapabilityFanout is advertised by hubs that understand one envelope per
// (webhook, destination) and echo delivery_id in their acks.
const CapabilityFanout = "fanout"

// CapabilityProxy is advertised by hubs that answer HttpRequest with
// HttpResponse (reverse proxy for the names in ConnectRequest.proxies).
const CapabilityProxy = proxy.Capability

// inFlightTimeout is how long a sent delivery is considered in flight without
// an ack before it may be sent again (forward timeout is 30s).
const inFlightTimeout = 90 * time.Second

// maxPendingProxy caps the proxied requests one hub may have in flight.
// Long-polls hold a slot for ~25s each, so it is well above the number of
// phones a household has open at once.
const maxPendingProxy = 128

// ConnectionManager manages multiple home-hub connections with endpoint routing.
type ConnectionManager struct {
	mu          sync.RWMutex
	connections map[string]*HubConnection // hubID → connection
	endpoints   map[string]string         // endpointID → hubID (routing table)
}

// HubConnection represents a single hub's connection state.
type HubConnection struct {
	hubID         string
	endpointIDs   []string
	capabilities  []string
	proxies       []string
	lastHeartbeat time.Time
	sendCh        chan *hooklyv1.WebhookEnvelope

	inFlightMu sync.Mutex
	inFlight   map[string]time.Time // deliveryID → sent at, until acked

	// Reverse proxy: requests wait here for the hub's response. proxyCh is
	// drained by the stream's send loop next to sendCh, so proxied requests
	// interleave with webhooks without reordering them.
	proxyCh   chan *hooklyv1.StreamResponse
	pendingMu sync.Mutex
	pending   map[string]chan *hooklyv1.HttpResponse // requestID → waiter
	closed    bool                                   // stream gone: fail every waiter
	done      chan struct{}                          // closed with the connection
}

// NewConnectionManager creates a new connection manager.
func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		connections: make(map[string]*HubConnection),
		endpoints:   make(map[string]string),
	}
}

// EndpointConflict is returned by TryAddConnection when another hub already
// relays one of the requested endpoints.
type EndpointConflict struct {
	EndpointID string
	HubID      string // the hub holding it
}

// AddConnection registers a new hub connection with its endpoints, taking
// them over from any other hub. Returns the HubConnection for sending webhooks.
func (m *ConnectionManager) AddConnection(hubID string, endpointIDs []string, capabilities []string) *HubConnection {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.addLocked(hubID, endpointIDs, capabilities)
}

// TryAddConnection registers a hub connection unless another live hub
// (heartbeat within staleAfter) already relays one of its endpoints: each
// endpoint goes to one hub, so a second relay for it would silently take all
// its deliveries. A reconnect from the same hub ID replaces the old connection.
//
// proxies are the names the hub reverse-proxies (only honoured with the
// "proxy" capability).
func (m *ConnectionManager) TryAddConnection(hubID string, endpointIDs []string, capabilities []string, proxies []string, staleAfter time.Duration) (*HubConnection, *EndpointConflict) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, epID := range endpointIDs {
		holder, ok := m.endpoints[epID]
		if !ok || holder == hubID {
			continue
		}
		if conn := m.connections[holder]; conn != nil && time.Since(conn.lastHeartbeat) <= staleAfter {
			return nil, &EndpointConflict{EndpointID: epID, HubID: holder}
		}
	}
	conn := m.addLocked(hubID, endpointIDs, capabilities)
	conn.proxies = proxies
	return conn, nil
}

func (m *ConnectionManager) addLocked(hubID string, endpointIDs []string, capabilities []string) *HubConnection {
	// Remove old connection if exists
	if old, exists := m.connections[hubID]; exists {
		m.unrouteLocked(old)
		close(old.sendCh)
		old.close()
	}

	conn := &HubConnection{
		hubID:         hubID,
		endpointIDs:   endpointIDs,
		capabilities:  capabilities,
		lastHeartbeat: time.Now(),
		sendCh:        make(chan *hooklyv1.WebhookEnvelope, 1000),
		inFlight:      make(map[string]time.Time),
		proxyCh:       make(chan *hooklyv1.StreamResponse, maxPendingProxy),
		pending:       make(map[string]chan *hooklyv1.HttpResponse),
		done:          make(chan struct{}),
	}

	m.connections[hubID] = conn

	// Register endpoint routing
	for _, epID := range endpointIDs {
		m.endpoints[epID] = hubID
	}

	slog.Info("hub connected",
		"hub_id", hubID,
		"endpoints", endpointIDs,
		"capabilities", capabilities,
		"total_hubs", len(m.connections),
	)

	return conn
}

// RemoveConnection removes a hub connection and its endpoint mappings.
// It is a no-op if the hub has already been replaced by a newer connection.
func (m *ConnectionManager) RemoveConnection(conn *HubConnection) {
	m.mu.Lock()
	defer m.mu.Unlock()

	hubID := conn.hubID
	if current, exists := m.connections[hubID]; !exists || current != conn {
		return
	}

	m.unrouteLocked(conn)
	delete(m.connections, hubID)
	conn.close()

	slog.Info("hub disconnected",
		"hub_id", hubID,
		"total_hubs", len(m.connections),
	)
}

// unrouteLocked removes conn's endpoint routes, leaving any that another hub
// has since taken over.
func (m *ConnectionManager) unrouteLocked(conn *HubConnection) {
	for _, epID := range conn.endpointIDs {
		if m.endpoints[epID] == conn.hubID {
			delete(m.endpoints, epID)
		}
	}
}

// GetHubForEndpoint returns the connection for the hub handling this endpoint.
// Returns nil if no hub handles this endpoint.
func (m *ConnectionManager) GetHubForEndpoint(endpointID string) *HubConnection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hubID, exists := m.endpoints[endpointID]
	if !exists {
		return nil
	}

	return m.connections[hubID]
}

// FindProxy returns the live connection of hubID if it advertised the proxy
// capability and serves name, else nil. It satisfies proxy.HubFinder.
func (m *ConnectionManager) FindProxy(hubID, name string) proxy.Hub {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conn, exists := m.connections[hubID]
	if !exists || !conn.SupportsProxy() || !slices.Contains(conn.proxies, name) {
		return nil
	}
	return conn
}

// IsAnyConnected returns true if at least one hub is connected.
func (m *ConnectionManager) IsAnyConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.connections) > 0
}

// ConnectedEndpointIDs returns all endpoint IDs that have active relay connections.
func (m *ConnectionManager) ConnectedEndpointIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.endpoints))
	for epID := range m.endpoints {
		ids = append(ids, epID)
	}
	return ids
}

// UpdateHeartbeat updates the heartbeat time for a hub.
func (m *ConnectionManager) UpdateHeartbeat(hubID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if conn, exists := m.connections[hubID]; exists {
		conn.lastHeartbeat = time.Now()
	}
}

// IsStale returns true if the hub hasn't sent a heartbeat within the timeout.
func (m *ConnectionManager) IsStale(hubID string, timeout time.Duration) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conn, exists := m.connections[hubID]
	if !exists {
		return false
	}
	return time.Since(conn.lastHeartbeat) > timeout
}

// Send queues a webhook for delivery to a specific hub.
// Returns false if buffer is full.
func (c *HubConnection) Send(webhook *hooklyv1.WebhookEnvelope) bool {
	select {
	case c.sendCh <- webhook:
		return true
	default:
		slog.Warn("webhook buffer full, dropping",
			"hub_id", c.hubID,
			"webhook_id", webhook.Id,
		)
		return false
	}
}

// SupportsFanout reports whether the hub understands per-destination envelopes.
// Hubs that don't are only sent each endpoint's primary destination.
func (c *HubConnection) SupportsFanout() bool {
	return slices.Contains(c.capabilities, CapabilityFanout)
}

// SupportsProxy reports whether the hub answers HttpRequests.
func (c *HubConnection) SupportsProxy() bool {
	return slices.Contains(c.capabilities, CapabilityProxy)
}

// Proxies lists the names the hub reverse-proxies.
func (c *HubConnection) Proxies() []string {
	return c.proxies
}

// Proxy queues req for the hub and waits for the HttpResponse with the same
// request ID, or until ctx ends (the caller's timeout) or the stream drops
// (proxy.ErrDisconnected). More than maxPendingProxy waiters is proxy.ErrBusy.
func (c *HubConnection) Proxy(ctx context.Context, req *hooklyv1.HttpRequest) (*hooklyv1.HttpResponse, error) {
	waiter := make(chan *hooklyv1.HttpResponse, 1)

	c.pendingMu.Lock()
	if c.closed {
		c.pendingMu.Unlock()
		return nil, proxy.ErrDisconnected
	}
	if len(c.pending) >= maxPendingProxy {
		c.pendingMu.Unlock()
		return nil, proxy.ErrBusy
	}
	c.pending[req.RequestId] = waiter
	c.pendingMu.Unlock()
	defer c.forget(req.RequestId)

	msg := &hooklyv1.StreamResponse{
		Message: &hooklyv1.StreamResponse_HttpRequest{HttpRequest: req},
	}
	select {
	case c.proxyCh <- msg:
	case <-c.done:
		return nil, proxy.ErrDisconnected
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case resp, ok := <-waiter:
		if !ok {
			return nil, proxy.ErrDisconnected
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ProxyCh returns the proxied requests waiting to go down the stream.
func (c *HubConnection) ProxyCh() <-chan *hooklyv1.StreamResponse {
	return c.proxyCh
}

// ResolveProxy hands the hub's response to the waiting request. Responses
// for unknown or already timed-out requests are dropped.
func (c *HubConnection) ResolveProxy(resp *hooklyv1.HttpResponse) {
	c.pendingMu.Lock()
	waiter, ok := c.pending[resp.RequestId]
	if ok {
		delete(c.pending, resp.RequestId)
	}
	c.pendingMu.Unlock()
	if ok {
		waiter <- resp // buffered: never blocks the receive loop
	} else {
		slog.Debug("proxy response for unknown request", "hub_id", c.hubID, "request_id", resp.RequestId)
	}
}

// PendingProxy is how many proxied requests await a response.
func (c *HubConnection) PendingProxy() int {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	return len(c.pending)
}

func (c *HubConnection) forget(requestID string) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	delete(c.pending, requestID)
}

// close fails every waiting proxied request: the stream is gone.
func (c *HubConnection) close() {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	close(c.done)
	for id, waiter := range c.pending {
		close(waiter)
		delete(c.pending, id)
	}
}

// MarkInFlight records that a delivery was sent and is awaiting its ack.
func (c *HubConnection) MarkInFlight(deliveryID string) {
	c.inFlightMu.Lock()
	defer c.inFlightMu.Unlock()
	c.inFlight[deliveryID] = time.Now()
}

// IsInFlight reports whether a delivery was sent and has not been acked yet.
// Deliveries unacked for longer than inFlightTimeout are no longer in flight.
func (c *HubConnection) IsInFlight(deliveryID string) bool {
	c.inFlightMu.Lock()
	defer c.inFlightMu.Unlock()
	sentAt, ok := c.inFlight[deliveryID]
	if ok && time.Since(sentAt) > inFlightTimeout {
		delete(c.inFlight, deliveryID)
		return false
	}
	return ok
}

// ClearInFlight is called when a delivery has been acked (or could not be queued).
func (c *HubConnection) ClearInFlight(deliveryID string) {
	c.inFlightMu.Lock()
	defer c.inFlightMu.Unlock()
	delete(c.inFlight, deliveryID)
}

// SendCh returns the channel for sending webhooks to this hub.
func (c *HubConnection) SendCh() <-chan *hooklyv1.WebhookEnvelope {
	return c.sendCh
}

// HubID returns the hub's identifier.
func (c *HubConnection) HubID() string {
	return c.hubID
}
