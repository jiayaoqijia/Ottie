package gateway

import (
	"testing"
)

func newTestLimiter() *RateLimiter {
	rl := NewRateLimiter(RateLimitConfig{
		MaxAttempts:    3,
		WindowSeconds:  60,
		LockoutSeconds: 300,
	})
	return rl
}

func TestAllowUnderLimit(t *testing.T) {
	rl := newTestLimiter()
	defer rl.Stop()

	for i := 0; i < 3; i++ {
		if !rl.Allow("login", "192.168.1.1") {
			t.Fatalf("expected Allow to return true on attempt %d", i+1)
		}
	}
}

func TestBlockAfterLimit(t *testing.T) {
	rl := newTestLimiter()
	defer rl.Stop()

	for i := 0; i < 3; i++ {
		rl.Allow("login", "192.168.1.1")
	}

	if rl.Allow("login", "192.168.1.1") {
		t.Fatal("expected Allow to return false after exceeding max attempts")
	}
}

func TestLoopbackAlwaysAllowed(t *testing.T) {
	rl := newTestLimiter()
	defer rl.Stop()

	loopbacks := []string{"127.0.0.1", "::1", "localhost", "127.0.0.1:8080"}
	for _, ip := range loopbacks {
		for i := 0; i < 10; i++ {
			if !rl.Allow("login", ip) {
				t.Fatalf("expected loopback %s to always be allowed", ip)
			}
		}
	}
}

func TestDifferentScopesAreIndependent(t *testing.T) {
	rl := newTestLimiter()
	defer rl.Stop()

	ip := "10.0.0.1"
	for i := 0; i < 3; i++ {
		rl.Allow("login", ip)
	}

	// login scope should be blocked
	if rl.Allow("login", ip) {
		t.Fatal("expected login scope to be blocked")
	}

	// register scope should still be allowed
	if !rl.Allow("register", ip) {
		t.Fatal("expected register scope to still be allowed")
	}
}

func TestDifferentIPsAreIndependent(t *testing.T) {
	rl := newTestLimiter()
	defer rl.Stop()

	for i := 0; i < 3; i++ {
		rl.Allow("login", "10.0.0.1")
	}

	if rl.Allow("login", "10.0.0.1") {
		t.Fatal("expected 10.0.0.1 to be blocked")
	}

	if !rl.Allow("login", "10.0.0.2") {
		t.Fatal("expected 10.0.0.2 to still be allowed")
	}
}
