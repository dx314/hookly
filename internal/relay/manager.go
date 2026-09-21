package relay

import (
	"log/slog"
	"slices"
	"sync"
	"time"

	hooklyv1 "hooks.dx314.com/internal/api/hookly/v1"
)

// CapabilityFanout is advertised by hubs that understand one envelope per
// (webhook, destination) and echo delivery_id in their acks.
const CapabilityFanout = "fanout"

// inFlightTimeout is how long a sent delivery is considered in flight without
// an ack before it may be sent again (forward timeout is 30s).
const inFlightTimeout = 90 * time.Second

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
	lastHeartbeat time.Time
	sendCh        chan *hooklyv1.WebhookEnvelope

	inFlightMu sync.Mutex
	inFlight   map[string]time.Time // deliveryID → sent at, until acked
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
func (m *ConnectionManager) TryAddConnection(hubID string, endpointIDs []string, capabilities []string, staleAfter time.Duration) (*HubConnection, *EndpointConflict) {
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
	return m.addLocked(hubID, endpointIDs, capabilities), nil
}

func (m *ConnectionManager) addLocked(hubID string, endpointIDs []string, capabilities []string) *HubConnection {
	// Remove old connection if exists
	if old, exists := m.connections[hubID]; exists {
		m.unrouteLocked(old)
		close(old.sendCh)
	}

	conn := &HubConnection{
		hubID:         hubID,
		endpointIDs:   endpointIDs,
		capabilities:  capabilities,
		lastHeartbeat: time.Now(),
		sendCh:        make(chan *hooklyv1.WebhookEnvelope, 1000),
		inFlight:      make(map[string]time.Time),
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
