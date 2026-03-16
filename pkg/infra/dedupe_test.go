package infra

import (
	"fmt"
	"testing"
	"time"
)

func TestDedupeNewKeyNotDuplicate(t *testing.T) {
	dc := NewDedupeCache(time.Minute, 100)
	if dc.Check("telegram|user1|chat1|msg1") {
		t.Fatal("expected new key to not be duplicate")
	}
}

func TestDedupeSameKeyIsDuplicate(t *testing.T) {
	dc := NewDedupeCache(time.Minute, 100)
	key := "telegram|user1|chat1|msg1"
	dc.Check(key)
	if !dc.Check(key) {
		t.Fatal("expected same key to be duplicate")
	}
}

func TestDedupeExpiredKeyNotDuplicate(t *testing.T) {
	dc := NewDedupeCache(10*time.Millisecond, 100)
	key := "telegram|user1|chat1|msg1"
	dc.Check(key)
	time.Sleep(20 * time.Millisecond)
	if dc.Check(key) {
		t.Fatal("expected expired key to not be duplicate")
	}
}

func TestDedupeMaxSizeEnforced(t *testing.T) {
	dc := NewDedupeCache(time.Minute, 5)
	for i := 0; i < 10; i++ {
		dc.Check(fmt.Sprintf("ch|user|chat|msg%d", i))
	}
	if dc.Len() > 5 {
		t.Fatalf("expected at most 5 entries, got %d", dc.Len())
	}
}

func TestDedupeDefaultValues(t *testing.T) {
	dc := NewDedupeCache(0, 0)
	if dc.ttl != defaultDedupeTTL {
		t.Fatalf("expected default TTL %v, got %v", defaultDedupeTTL, dc.ttl)
	}
	if dc.maxSize != defaultDedupeMaxSize {
		t.Fatalf("expected default maxSize %d, got %d", defaultDedupeMaxSize, dc.maxSize)
	}
}
