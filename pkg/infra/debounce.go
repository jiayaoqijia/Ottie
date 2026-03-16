package infra

import (
	"sync"
	"time"

	"github.com/jiayaoqijia/ottie/pkg/bus"
)

type debounceEntry struct {
	timer *time.Timer
	msg   bus.InboundMessage
}

// Debouncer groups rapid messages by key (channel:chatID) and flushes
// only the latest message after a configurable delay. A delay of 0 disables
// debouncing and invokes the callback immediately.
type Debouncer struct {
	mu       sync.Mutex
	delay    time.Duration
	callback func(bus.InboundMessage)
	entries  map[string]*debounceEntry
	stopped  bool
}

// NewDebouncer creates a new Debouncer. The callback is invoked with the
// latest message for each key after the delay elapses. A zero delay disables
// debouncing — callback fires immediately on each Submit.
func NewDebouncer(delay time.Duration, callback func(bus.InboundMessage)) *Debouncer {
	return &Debouncer{
		delay:    delay,
		callback: callback,
		entries:  make(map[string]*debounceEntry),
	}
}

// Submit stores the latest message for the given key and resets the timer.
// When the timer expires, the callback is invoked with the most recent message.
func (d *Debouncer) Submit(key string, msg bus.InboundMessage) {
	if d.delay <= 0 {
		d.callback(msg)
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stopped {
		return
	}

	if e, ok := d.entries[key]; ok {
		e.timer.Stop()
		e.msg = msg
		e.timer = time.AfterFunc(d.delay, func() {
			d.flush(key)
		})
		return
	}

	d.entries[key] = &debounceEntry{
		msg: msg,
		timer: time.AfterFunc(d.delay, func() {
			d.flush(key)
		}),
	}
}

// Stop cancels all pending timers.
func (d *Debouncer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.stopped = true
	for key, e := range d.entries {
		e.timer.Stop()
		delete(d.entries, key)
	}
}

func (d *Debouncer) flush(key string) {
	d.mu.Lock()
	e, ok := d.entries[key]
	if !ok {
		d.mu.Unlock()
		return
	}
	msg := e.msg
	delete(d.entries, key)
	d.mu.Unlock()

	d.callback(msg)
}
