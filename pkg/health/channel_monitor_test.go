package health

import (
	"context"
	"testing"
	"time"
)

func newTestMonitor() *ChannelMonitor {
	m := NewChannelMonitor(5*time.Minute, 2*time.Minute)
	return m
}

func TestNewChannelMonitor(t *testing.T) {
	m := NewChannelMonitor(5*time.Minute, 2*time.Minute)
	if m.checkInterval != 5*time.Minute {
		t.Errorf("expected checkInterval 5m, got %v", m.checkInterval)
	}
	if m.startupGrace != 2*time.Minute {
		t.Errorf("expected startupGrace 2m, got %v", m.startupGrace)
	}
	if m.staleThreshold != 30*time.Minute {
		t.Errorf("expected staleThreshold 30m, got %v", m.staleThreshold)
	}
	if m.stuckThreshold != 25*time.Minute {
		t.Errorf("expected stuckThreshold 25m, got %v", m.stuckThreshold)
	}
}

func TestStatusUnknownChannel(t *testing.T) {
	m := newTestMonitor()
	if s := m.Status("nonexistent"); s != StatusNotRunning {
		t.Errorf("expected StatusNotRunning for unknown channel, got %v", s)
	}
}

func TestStatusNotRunning(t *testing.T) {
	m := newTestMonitor()
	m.SetRunning("slack", false)
	if s := m.Status("slack"); s != StatusNotRunning {
		t.Errorf("expected StatusNotRunning, got %v", s)
	}
}

func TestStatusHealthyAfterStart(t *testing.T) {
	now := time.Now()
	m := newTestMonitor()
	m.now = func() time.Time { return now }

	m.RecordStart("slack")
	if s := m.Status("slack"); s != StatusHealthy {
		t.Errorf("expected StatusHealthy after start, got %v", s)
	}
}

func TestStatusHealthyWithinStartupGrace(t *testing.T) {
	now := time.Now()
	m := newTestMonitor()
	m.now = func() time.Time { return now }
	m.RecordStart("slack")

	// Advance to 1 minute (within 2min grace).
	m.now = func() time.Time { return now.Add(1 * time.Minute) }
	if s := m.Status("slack"); s != StatusHealthy {
		t.Errorf("expected StatusHealthy within grace period, got %v", s)
	}
}

func TestStatusHealthyWithRecentEvents(t *testing.T) {
	now := time.Now()
	m := newTestMonitor()
	m.now = func() time.Time { return now }
	m.RecordStart("slack")

	// Move past startup grace, but keep sending events.
	m.now = func() time.Time { return now.Add(10 * time.Minute) }
	m.RecordEvent("slack")

	m.now = func() time.Time { return now.Add(15 * time.Minute) }
	if s := m.Status("slack"); s != StatusHealthy {
		t.Errorf("expected StatusHealthy with recent events, got %v", s)
	}
}

func TestStatusStaleSocket(t *testing.T) {
	now := time.Now()
	m := newTestMonitor()
	m.now = func() time.Time { return now }
	m.RecordStart("slack")

	// Advance 31 minutes with no events.
	m.now = func() time.Time { return now.Add(31 * time.Minute) }
	if s := m.Status("slack"); s != StatusStaleSocket {
		t.Errorf("expected StatusStaleSocket, got %v", s)
	}
}

func TestStatusStuck(t *testing.T) {
	now := time.Now()
	m := newTestMonitor()
	m.now = func() time.Time { return now }
	m.RecordStart("slack")

	// Set busy and advance 26 minutes.
	m.SetBusy("slack", true)
	m.now = func() time.Time { return now.Add(26 * time.Minute) }

	if s := m.Status("slack"); s != StatusStuck {
		t.Errorf("expected StatusStuck, got %v", s)
	}
}

func TestBusyButNotStuck(t *testing.T) {
	now := time.Now()
	m := newTestMonitor()
	m.now = func() time.Time { return now }
	m.RecordStart("slack")

	m.SetBusy("slack", true)
	// 10 minutes — busy but under threshold.
	m.now = func() time.Time { return now.Add(10 * time.Minute) }
	if s := m.Status("slack"); s != StatusHealthy {
		t.Errorf("expected StatusHealthy when busy but under threshold, got %v", s)
	}
}

func TestStuckTakesPriorityOverStale(t *testing.T) {
	now := time.Now()
	m := newTestMonitor()
	m.now = func() time.Time { return now }
	m.RecordStart("slack")
	m.SetBusy("slack", true)

	// 31 min — exceeds both thresholds; stuck is checked first.
	m.now = func() time.Time { return now.Add(31 * time.Minute) }
	if s := m.Status("slack"); s != StatusStuck {
		t.Errorf("expected StatusStuck (priority over stale), got %v", s)
	}
}

func TestNotBusyGoesStaleNotStuck(t *testing.T) {
	now := time.Now()
	m := newTestMonitor()
	m.now = func() time.Time { return now }
	m.RecordStart("slack")

	// Not busy, 31 min → stale, not stuck.
	m.now = func() time.Time { return now.Add(31 * time.Minute) }
	if s := m.Status("slack"); s != StatusStaleSocket {
		t.Errorf("expected StatusStaleSocket when not busy, got %v", s)
	}
}

func TestGracePeriodPreventsStale(t *testing.T) {
	now := time.Now()
	m := NewChannelMonitor(5*time.Minute, 45*time.Minute)
	m.now = func() time.Time { return now }
	m.RecordStart("slack")

	// 31 min — would be stale, but grace is 45min.
	m.now = func() time.Time { return now.Add(31 * time.Minute) }
	if s := m.Status("slack"); s != StatusHealthy {
		t.Errorf("expected StatusHealthy during extended grace period, got %v", s)
	}
}

func TestRecordEventResetsStale(t *testing.T) {
	now := time.Now()
	m := newTestMonitor()
	m.now = func() time.Time { return now }
	m.RecordStart("slack")

	// Advance to near-stale, then record event.
	m.now = func() time.Time { return now.Add(29 * time.Minute) }
	m.RecordEvent("slack")

	// Another 10 min after event — total 39 min from start but only 10 from last event.
	m.now = func() time.Time { return now.Add(39 * time.Minute) }
	if s := m.Status("slack"); s != StatusHealthy {
		t.Errorf("expected StatusHealthy after event reset, got %v", s)
	}
}

func TestSetRunningFalseAfterStart(t *testing.T) {
	now := time.Now()
	m := newTestMonitor()
	m.now = func() time.Time { return now }
	m.RecordStart("slack")
	m.SetRunning("slack", false)

	if s := m.Status("slack"); s != StatusNotRunning {
		t.Errorf("expected StatusNotRunning after SetRunning(false), got %v", s)
	}
}

func TestAllStatuses(t *testing.T) {
	now := time.Now()
	m := newTestMonitor()
	m.now = func() time.Time { return now }

	m.RecordStart("slack")
	m.RecordStart("discord")
	m.SetRunning("discord", false)

	statuses := m.AllStatuses()
	if len(statuses) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(statuses))
	}
	if statuses["slack"] != StatusHealthy {
		t.Errorf("expected slack healthy, got %v", statuses["slack"])
	}
	if statuses["discord"] != StatusNotRunning {
		t.Errorf("expected discord not-running, got %v", statuses["discord"])
	}
}

func TestHealthCheckAllHealthy(t *testing.T) {
	now := time.Now()
	m := newTestMonitor()
	m.now = func() time.Time { return now }
	m.RecordStart("slack")
	m.RecordStart("discord")

	ok, msg := m.HealthCheck()()
	if !ok {
		t.Errorf("expected healthy check, got false: %s", msg)
	}
	if msg != "all channels healthy" {
		t.Errorf("unexpected message: %s", msg)
	}
}

func TestHealthCheckStuckChannel(t *testing.T) {
	now := time.Now()
	m := newTestMonitor()
	m.now = func() time.Time { return now }
	m.RecordStart("slack")
	m.SetBusy("slack", true)

	m.now = func() time.Time { return now.Add(26 * time.Minute) }

	ok, msg := m.HealthCheck()()
	if ok {
		t.Errorf("expected unhealthy check for stuck channel")
	}
	if msg != "channel slack: stuck" {
		t.Errorf("unexpected message: %s", msg)
	}
}

func TestHealthCheckStaleChannel(t *testing.T) {
	now := time.Now()
	m := newTestMonitor()
	m.now = func() time.Time { return now }
	m.RecordStart("slack")

	m.now = func() time.Time { return now.Add(31 * time.Minute) }

	ok, msg := m.HealthCheck()()
	if ok {
		t.Errorf("expected unhealthy check for stale channel")
	}
	if msg != "channel slack: stale-socket" {
		t.Errorf("unexpected message: %s", msg)
	}
}

func TestHealthCheckNotRunningIsOK(t *testing.T) {
	m := newTestMonitor()
	m.SetRunning("slack", false)

	ok, msg := m.HealthCheck()()
	if !ok {
		t.Errorf("expected healthy check (not-running is not a failure), got: %s", msg)
	}
}

func TestRecordStartIncrementsRestartCount(t *testing.T) {
	now := time.Now()
	m := newTestMonitor()
	m.now = func() time.Time { return now }

	m.RecordStart("slack")
	m.mu.RLock()
	count := m.channels["slack"].restartCount
	m.mu.RUnlock()
	if count != 0 {
		t.Errorf("expected restartCount 0 on first start, got %d", count)
	}

	m.RecordStart("slack")
	m.mu.RLock()
	count = m.channels["slack"].restartCount
	m.mu.RUnlock()
	if count != 1 {
		t.Errorf("expected restartCount 1 after second start, got %d", count)
	}
}

func TestStartContextCancellation(t *testing.T) {
	m := NewChannelMonitor(50*time.Millisecond, 2*time.Minute)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		m.Start(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}

func TestSetBusyFalseRecovery(t *testing.T) {
	now := time.Now()
	m := newTestMonitor()
	m.now = func() time.Time { return now }
	m.RecordStart("slack")
	m.SetBusy("slack", true)

	// Would be stuck...
	m.now = func() time.Time { return now.Add(26 * time.Minute) }
	if s := m.Status("slack"); s != StatusStuck {
		t.Fatalf("expected stuck, got %v", s)
	}

	// Clear busy + record event → healthy again.
	m.SetBusy("slack", false)
	m.RecordEvent("slack")
	if s := m.Status("slack"); s != StatusHealthy {
		t.Errorf("expected StatusHealthy after clearing busy and recording event, got %v", s)
	}
}
