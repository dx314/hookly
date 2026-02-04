package relay

import (
	"log/slog"
	"sync"
	"time"

	hooklyv1 "hookly/internal/api/hookly/v1"
)

// ConnectionManager manages the active home-hub connection.
// Currently supports a single connection (no multi-tenant).
type ConnectionManager struct {
	mu            sync.RWMutex
	connected     bool
	lastHeartbeat time.Time
	sendCh        chan *hooklyv1.WebhookEnvelope
	hubID         string
}

// NewConnectionManager creates a new connection manager.
func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		sendCh: make(chan *hooklyv1.WebhookEnvelope, 1000), // Buffer up to 1000 webhooks
	}
}

// SetConnected marks the connection as active.
func (m *ConnectionManager) SetConnected(hubID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = true
	m.hubID = hubID
	m.lastHeartbeat = time.Now()
	slog.Info("home-hub connected", "hub_id", hubID)
}

// SetDisconnected marks the connection as inactive.
func (m *ConnectionManager) SetDisconnected() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.connected {
		slog.Info("home-hub disconnected", "hub_id", m.hubID)
	}
	m.connected = false
	m.hubID = ""
}

// IsConnected returns true if a home-hub is connected.
func (m *ConnectionManager) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

// UpdateHeartbeat updates the last heartbeat time.
func (m *ConnectionManager) UpdateHeartbeat() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastHeartbeat = time.Now()
}

// LastHeartbeat returns the last heartbeat time.
func (m *ConnectionManager) LastHeartbeat() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastHeartbeat
}

// IsStale returns true if no heartbeat received in the given duration.
func (m *ConnectionManager) IsStale(timeout time.Duration) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.connected {
		return false
	}
	return time.Since(m.lastHeartbeat) > timeout
}

// SendCh returns the channel for sending webhooks to home-hub.
func (m *ConnectionManager) SendCh() chan *hooklyv1.WebhookEnvelope {
	return m.sendCh
}

// Send queues a webhook for delivery to home-hub.
// Returns false if buffer is full.
func (m *ConnectionManager) Send(webhook *hooklyv1.WebhookEnvelope) bool {
	select {
	case m.sendCh <- webhook:
		return true
	default:
		slog.Warn("webhook buffer full, dropping", "webhook_id", webhook.Id)
		return false
	}
}
