package infra

import (
	"sync"
	"testing"
	"time"

	"github.com/jiayaoqijia/ottie/pkg/bus"
)

func TestDebouncerFlushesLatestMessage(t *testing.T) {
	var mu sync.Mutex
	var result bus.InboundMessage
	var count int

	d := NewDebouncer(50*time.Millisecond, func(msg bus.InboundMessage) {
		mu.Lock()
		result = msg
		count++
		mu.Unlock()
	})
	defer d.Stop()

	d.Submit("telegram:chat1", bus.InboundMessage{Content: "first"})
	d.Submit("telegram:chat1", bus.InboundMessage{Content: "second"})
	d.Submit("telegram:chat1", bus.InboundMessage{Content: "third"})

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if count != 1 {
		t.Fatalf("expected callback to fire once, got %d", count)
	}
	if result.Content != "third" {
		t.Fatalf("expected last message 'third', got %q", result.Content)
	}
}

func TestDebouncerZeroDelayFiresImmediately(t *testing.T) {
	var count int
	d := NewDebouncer(0, func(_ bus.InboundMessage) {
		count++
	})
	defer d.Stop()

	d.Submit("key", bus.InboundMessage{Content: "a"})
	d.Submit("key", bus.InboundMessage{Content: "b"})

	if count != 2 {
		t.Fatalf("expected 2 immediate calls, got %d", count)
	}
}

func TestDebouncerDifferentKeysFlushIndependently(t *testing.T) {
	var mu sync.Mutex
	results := make(map[string]string)

	d := NewDebouncer(50*time.Millisecond, func(msg bus.InboundMessage) {
		mu.Lock()
		results[msg.ChatID] = msg.Content
		mu.Unlock()
	})
	defer d.Stop()

	d.Submit("telegram:chat1", bus.InboundMessage{ChatID: "chat1", Content: "msg1"})
	d.Submit("telegram:chat2", bus.InboundMessage{ChatID: "chat2", Content: "msg2"})

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results["chat1"] != "msg1" {
		t.Fatalf("expected chat1='msg1', got %q", results["chat1"])
	}
	if results["chat2"] != "msg2" {
		t.Fatalf("expected chat2='msg2', got %q", results["chat2"])
	}
}

func TestDebouncerStopCancelsPending(t *testing.T) {
	var count int
	d := NewDebouncer(50*time.Millisecond, func(_ bus.InboundMessage) {
		count++
	})

	d.Submit("key", bus.InboundMessage{Content: "hello"})
	d.Stop()

	time.Sleep(100 * time.Millisecond)

	if count != 0 {
		t.Fatalf("expected no callbacks after stop, got %d", count)
	}
}
