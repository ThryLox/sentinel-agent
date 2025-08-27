package bus

import (
	"log"
	"sentinel-agent/internal/events"
	"sync"
)

// Bus manages the event pipeline.
type Bus struct {
	subscribers []chan events.Event
	mu          sync.RWMutex
}

// New creates a new Event Bus.
func New() *Bus {
	return &Bus{
		subscribers: make([]chan events.Event, 0),
	}
}

// Subscribe returns a read-only channel to receive all events.
// The channel is buffered (size 100) to prevent blocking publishers.
func (b *Bus) Subscribe() <-chan events.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan events.Event, 1000)
	b.subscribers = append(b.subscribers, ch)
	return ch
}

// Publish broadcasts an event to all subscribers.
// It is non-blocking; if a subscriber is slow/full, the event is dropped (or we could block/expand).
// For this security agent, we chose to drop if full to avoid stalling the pipeline.
func (b *Bus) Publish(e events.Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers {
		select {
		case ch <- e:
		default:
			// Drop or log warning
			log.Printf("WARNING: Bus dropped event type %s due to full buffer", e.Type)
		}
	}
}
