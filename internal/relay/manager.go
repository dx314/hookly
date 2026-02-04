package relay

import (
	"log/slog"
	"sync"
	"time"

	hooklyv1 "hooks.dx314.com/internal/api/hookly/v1"
)

// ConnectionManager manages multiple home-hub connections with endpoint routing.
type ConnectionManager struct {
	mu          sync.RWMutex
	connections map[string]*HubConnection  // hubID → connection
	endpoints   map[string]string          // endpointID → hubID (routing table)
}

// HubConnection represents a single hub's connection state.
type HubConnection struct {
	hubID         string
	endpointIDs   []string
	lastHeartbeat time.Time
	sendCh        chan *hooklyv1.WebhookEnvelope
}

// NewConnectionManager creates a new connection manager.
func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		connections: make(map[string]*HubConnection),
		endpoints:   make(map[string]string),
	}
}

// AddConnection registers a new hub connection with its endpoints.
// Returns the HubConnection for sending webhooks.
func (m *ConnectionManager) AddConnection(hubID string, endpointIDs []string) *HubConnection {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Remove old connection if exists
	if old, exists := m.connections[hubID]; exists {
		for _, epID := range old.endpointIDs {
			delete(m.endpoints, epID)
		}
		close(old.sendCh)
	}

	conn := &HubConnection{
		hubID:         hubID,
		endpointIDs:   endpointIDs,
		lastHeartbeat: time.Now(),
		sendCh:        make(chan *hooklyv1.WebhookEnvelope, 1000),
	}

	m.connections[hubID] = conn

	// Register endpoint routing
	for _, epID := range endpointIDs {
		m.endpoints[epID] = hubID
	}

	slog.Info("hub connected",
		"hub_id", hubID,
		"endpoints", endpointIDs,
		"total_hubs", len(m.connections),
	)

	return conn
}

// RemoveConnection removes a hub and its endpoint mappings.
func (m *ConnectionManager) RemoveConnection(hubID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	conn, exists := m.connections[hubID]
	if !exists {
		return
	}

	// Remove endpoint mappings
	for _, epID := range conn.endpointIDs {
		delete(m.endpoints, epID)
	}

	delete(m.connections, hubID)

	slog.Info("hub disconnected",
		"hub_id", hubID,
		"total_hubs", len(m.connections),
	)
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

// SendCh returns the channel for sending webhooks to this hub.
func (c *HubConnection) SendCh() <-chan *hooklyv1.WebhookEnvelope {
	return c.sendCh
}

// HubID returns the hub's identifier.
func (c *HubConnection) HubID() string {
	return c.hubID
}
