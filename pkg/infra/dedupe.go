package infra

import (
	"sync"
	"time"
)

const (
	defaultDedupeTTL     = 20 * time.Minute
	defaultDedupeMaxSize = 5000
)

type dedupeEntry struct {
	createdAt time.Time
}

// DedupeCache is a TTL+maxSize map cache for message deduplication.
// Key format: channel|senderID|chatID|messageID.
type DedupeCache struct {
	mu      sync.Mutex
	entries map[string]*dedupeEntry
	ttl     time.Duration
	maxSize int
}

// NewDedupeCache creates a new DedupeCache with the given TTL and max size.
// Zero values use defaults (20 min TTL, 5000 max size).
func NewDedupeCache(ttl time.Duration, maxSize int) *DedupeCache {
	if ttl <= 0 {
		ttl = defaultDedupeTTL
	}
	if maxSize <= 0 {
		maxSize = defaultDedupeMaxSize
	}
	return &DedupeCache{
		entries: make(map[string]*dedupeEntry),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

// Check returns true if the key is a duplicate (already seen within TTL).
// If the key is new, it is inserted and false is returned.
func (dc *DedupeCache) Check(key string) bool {
	now := time.Now()

	dc.mu.Lock()
	defer dc.mu.Unlock()

	if e, ok := dc.entries[key]; ok {
		if now.Sub(e.createdAt) < dc.ttl {
			return true
		}
		// Expired entry — treat as new.
		e.createdAt = now
		dc.prune(now)
		return false
	}

	dc.entries[key] = &dedupeEntry{createdAt: now}
	dc.prune(now)
	return false
}

// Len returns the number of entries (for testing).
func (dc *DedupeCache) Len() int {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	return len(dc.entries)
}

// prune removes expired entries and evicts oldest if over maxSize.
// Must be called with dc.mu held.
func (dc *DedupeCache) prune(now time.Time) {
	// Remove expired entries.
	for k, e := range dc.entries {
		if now.Sub(e.createdAt) >= dc.ttl {
			delete(dc.entries, k)
		}
	}

	// If still over capacity, evict oldest entries.
	for len(dc.entries) > dc.maxSize {
		var oldestKey string
		var oldestTime time.Time
		first := true
		for k, e := range dc.entries {
			if first || e.createdAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = e.createdAt
				first = false
			}
		}
		delete(dc.entries, oldestKey)
	}
}
