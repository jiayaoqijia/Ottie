package health

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type ChannelStatus string

const (
	StatusHealthy      ChannelStatus = "healthy"
	StatusNotRunning   ChannelStatus = "not-running"
	StatusDisconnected ChannelStatus = "disconnected"
	StatusStaleSocket  ChannelStatus = "stale-socket"
	StatusStuck        ChannelStatus = "stuck"
)

const (
	defaultStaleThreshold = 30 * time.Minute
	defaultStuckThreshold = 25 * time.Minute
)

type channelState struct {
	running      bool
	connected    bool
	busy         bool
	lastEventAt  time.Time
	lastStartAt  time.Time
	restartCount int
}

type ChannelMonitor struct {
	mu             sync.RWMutex
	channels       map[string]*channelState
	checkInterval  time.Duration
	startupGrace   time.Duration
	staleThreshold time.Duration
	stuckThreshold time.Duration
	now            func() time.Time // for testing
}

func NewChannelMonitor(checkInterval, startupGrace time.Duration) *ChannelMonitor {
	return &ChannelMonitor{
		channels:       make(map[string]*channelState),
		checkInterval:  checkInterval,
		startupGrace:   startupGrace,
		staleThreshold: defaultStaleThreshold,
		stuckThreshold: defaultStuckThreshold,
		now:            time.Now,
	}
}

func (m *ChannelMonitor) getOrCreate(channel string) *channelState {
	s, ok := m.channels[channel]
	if !ok {
		s = &channelState{}
		m.channels[channel] = s
	}
	return s
}

// RecordEvent marks that an event was received on a channel.
func (m *ChannelMonitor) RecordEvent(channel string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.getOrCreate(channel)
	s.lastEventAt = m.now()
}

// RecordStart marks that a channel has started (or restarted).
func (m *ChannelMonitor) RecordStart(channel string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.getOrCreate(channel)
	now := m.now()
	if !s.lastStartAt.IsZero() {
		s.restartCount++
	}
	s.lastStartAt = now
	s.lastEventAt = now
	s.running = true
	s.connected = true
}

// SetRunning sets the running state for a channel.
func (m *ChannelMonitor) SetRunning(channel string, running bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.getOrCreate(channel)
	s.running = running
}

// SetBusy sets the busy state for a channel (processing a message).
func (m *ChannelMonitor) SetBusy(channel string, busy bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.getOrCreate(channel)
	s.busy = busy
}

// Status returns the health status for a single channel.
func (m *ChannelMonitor) Status(channel string) ChannelStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.channels[channel]
	if !ok {
		return StatusNotRunning
	}
	return m.statusForState(s)
}

func (m *ChannelMonitor) statusForState(s *channelState) ChannelStatus {
	if !s.running {
		return StatusNotRunning
	}

	now := m.now()

	// Within startup grace period, consider healthy.
	if now.Sub(s.lastStartAt) < m.startupGrace {
		return StatusHealthy
	}

	timeSinceEvent := now.Sub(s.lastEventAt)

	// Busy and no events for >stuckThreshold → stuck.
	if s.busy && timeSinceEvent > m.stuckThreshold {
		return StatusStuck
	}

	// No events for >staleThreshold → stale socket.
	if timeSinceEvent > m.staleThreshold {
		return StatusStaleSocket
	}

	return StatusHealthy
}

// AllStatuses returns statuses for all known channels.
func (m *ChannelMonitor) AllStatuses() map[string]ChannelStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]ChannelStatus, len(m.channels))
	for name, s := range m.channels {
		result[name] = m.statusForState(s)
	}
	return result
}

// HealthCheck returns a function compatible with health.Server.RegisterCheck.
func (m *ChannelMonitor) HealthCheck() func() (bool, string) {
	return func() (bool, string) {
		statuses := m.AllStatuses()
		for name, status := range statuses {
			if status == StatusStuck || status == StatusStaleSocket {
				return false, fmt.Sprintf("channel %s: %s", name, status)
			}
		}
		return true, "all channels healthy"
	}
}

// Start begins periodic health checking (for logging/metrics).
func (m *ChannelMonitor) Start(ctx context.Context) {
	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			statuses := m.AllStatuses()
			for name, status := range statuses {
				if status != StatusHealthy {
					slog.Warn("channel unhealthy",
						"channel", name,
						"status", string(status),
					)
				}
			}
		}
	}
}
