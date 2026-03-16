package infra

import (
	"context"
	"testing"
	"time"
)

func TestBackoffDelayIncreases(t *testing.T) {
	p := BackoffPolicy{InitialMs: 100, MaxMs: 10000, Factor: 2.0, Jitter: 0}

	d1 := p.Delay(1)
	d2 := p.Delay(2)
	d3 := p.Delay(3)

	if d1 != 100*time.Millisecond {
		t.Fatalf("expected 100ms for attempt 1, got %v", d1)
	}
	if d2 != 200*time.Millisecond {
		t.Fatalf("expected 200ms for attempt 2, got %v", d2)
	}
	if d3 != 400*time.Millisecond {
		t.Fatalf("expected 400ms for attempt 3, got %v", d3)
	}
}

func TestBackoffDelayRespectsCap(t *testing.T) {
	p := BackoffPolicy{InitialMs: 100, MaxMs: 300, Factor: 2.0, Jitter: 0}

	d := p.Delay(10)
	if d != 300*time.Millisecond {
		t.Fatalf("expected capped at 300ms, got %v", d)
	}
}

func TestBackoffDelayWithJitter(t *testing.T) {
	p := BackoffPolicy{InitialMs: 1000, MaxMs: 10000, Factor: 2.0, Jitter: 0.1}

	for i := 0; i < 100; i++ {
		d := p.Delay(1)
		if d < 900*time.Millisecond || d > 1100*time.Millisecond {
			t.Fatalf("jittered delay out of range: %v", d)
		}
	}
}

func TestBackoffDelayZeroAttempt(t *testing.T) {
	p := BackoffPolicy{InitialMs: 100, MaxMs: 10000, Factor: 2.0, Jitter: 0}
	d := p.Delay(0)
	if d != 100*time.Millisecond {
		t.Fatalf("expected 100ms for attempt 0 (treated as 1), got %v", d)
	}
}

func TestBackoffSleepCancelledContext(t *testing.T) {
	p := BackoffPolicy{InitialMs: 5000, MaxMs: 10000, Factor: 2.0, Jitter: 0}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := p.Sleep(ctx, 1)
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestDefaultBackoffValues(t *testing.T) {
	p := DefaultBackoff()
	if p.InitialMs != 500 || p.MaxMs != 8000 || p.Factor != 2.0 || p.Jitter != 0.1 {
		t.Fatalf("unexpected default values: %+v", p)
	}
}
