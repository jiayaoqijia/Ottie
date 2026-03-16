package gateway

import (
	"net"
	"sync"
	"time"
)

// RateLimitConfig holds the configuration for the rate limiter.
type RateLimitConfig struct {
	MaxAttempts    int
	WindowSeconds  int
	LockoutSeconds int
}

type entry struct {
	timestamps  []time.Time
	lockedUntil time.Time
}

// RateLimiter tracks request rates per scope+IP and enforces limits.
type RateLimiter struct {
	mu      sync.Mutex
	entries map[string]*entry
	config  RateLimitConfig
	stopCh  chan struct{}
}

// NewRateLimiter creates a new RateLimiter with the given configuration
// and starts a background goroutine to prune expired entries.
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		entries: make(map[string]*entry),
		config:  cfg,
		stopCh:  make(chan struct{}),
	}
	go rl.pruneLoop()
	return rl
}

// Allow returns true if the request from the given IP in the given scope
// should be allowed. Loopback addresses are always allowed.
func (rl *RateLimiter) Allow(scope, ip string) bool {
	if isLoopback(ip) {
		return true
	}

	key := scope + ":" + ip
	now := time.Now()
	window := time.Duration(rl.config.WindowSeconds) * time.Second

	rl.mu.Lock()
	defer rl.mu.Unlock()

	e, ok := rl.entries[key]
	if !ok {
		e = &entry{}
		rl.entries[key] = e
	}

	// Check if currently locked out.
	if now.Before(e.lockedUntil) {
		return false
	}

	// Remove timestamps outside the sliding window.
	cutoff := now.Add(-window)
	filtered := e.timestamps[:0]
	for _, ts := range e.timestamps {
		if ts.After(cutoff) {
			filtered = append(filtered, ts)
		}
	}
	e.timestamps = filtered

	// Check if at the limit.
	if len(e.timestamps) >= rl.config.MaxAttempts {
		e.lockedUntil = now.Add(time.Duration(rl.config.LockoutSeconds) * time.Second)
		e.timestamps = nil
		return false
	}

	e.timestamps = append(e.timestamps, now)
	return true
}

// Stop stops the background prune goroutine.
func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

func (rl *RateLimiter) pruneLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
			rl.prune()
		}
	}
}

func (rl *RateLimiter) prune() {
	now := time.Now()
	window := time.Duration(rl.config.WindowSeconds) * time.Second

	rl.mu.Lock()
	defer rl.mu.Unlock()

	for key, e := range rl.entries {
		// Remove if lockout has expired and no recent timestamps.
		if now.After(e.lockedUntil) {
			cutoff := now.Add(-window)
			hasRecent := false
			for _, ts := range e.timestamps {
				if ts.After(cutoff) {
					hasRecent = true
					break
				}
			}
			if !hasRecent {
				delete(rl.entries, key)
			}
		}
	}
}

func isLoopback(ip string) bool {
	// Strip port if present.
	host, _, err := net.SplitHostPort(ip)
	if err == nil {
		ip = host
	}
	if ip == "localhost" {
		return true
	}
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.IsLoopback()
}
