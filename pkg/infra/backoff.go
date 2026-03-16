package infra

import (
	"context"
	"math"
	"math/rand/v2"
	"time"
)

// BackoffPolicy defines parameters for exponential backoff.
type BackoffPolicy struct {
	InitialMs int     // Base delay in milliseconds.
	MaxMs     int     // Maximum delay in milliseconds.
	Factor    float64 // Multiplicative factor per attempt.
	Jitter    float64 // Fraction of delay to randomize (0.0-1.0).
}

// DefaultBackoff returns a sensible default backoff policy.
func DefaultBackoff() BackoffPolicy {
	return BackoffPolicy{
		InitialMs: 500,
		MaxMs:     8000,
		Factor:    2.0,
		Jitter:    0.1,
	}
}

// Delay returns the backoff duration for the given attempt number (1-based).
// Formula: min(MaxMs, InitialMs * Factor^(attempt-1) + jitter)
func (p BackoffPolicy) Delay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}

	delayMs := float64(p.InitialMs) * math.Pow(p.Factor, float64(attempt-1))
	if delayMs > float64(p.MaxMs) {
		delayMs = float64(p.MaxMs)
	}

	if p.Jitter > 0 {
		jitterAmount := delayMs * p.Jitter
		delayMs += jitterAmount * (2*rand.Float64() - 1)
	}

	if delayMs < 0 {
		delayMs = 0
	}

	return time.Duration(delayMs) * time.Millisecond
}

// Sleep blocks for the backoff duration of the given attempt.
// It returns ctx.Err() if the context is canceled before the delay elapses.
func (p BackoffPolicy) Sleep(ctx context.Context, attempt int) error {
	d := p.Delay(attempt)
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
