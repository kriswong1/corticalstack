// Package sse provides a minimal pub/sub event bus for server-sent events.
package sse

import (
	"encoding/json"
	"sync"
)

// Event is a typed message pushed to browser clients via SSE.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// EventBus is a simple pub/sub for SSE events.
type EventBus struct {
	subscribers map[chan Event]struct{}
	mu          sync.RWMutex
}

// NewEventBus creates an event bus.
func NewEventBus() *EventBus {
	return &EventBus{subscribers: make(map[chan Event]struct{})}
}

// Subscribe returns a buffered channel that receives events.
// The caller must Unsubscribe when done.
func (b *EventBus) Subscribe() chan Event {
	ch := make(chan Event, 64)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber and closes its channel.
func (b *EventBus) Unsubscribe(ch chan Event) {
	b.mu.Lock()
	if _, ok := b.subscribers[ch]; ok {
		delete(b.subscribers, ch)
		close(ch)
	}
	b.mu.Unlock()
}

// Publish sends an event to all subscribers without blocking.
// Subscribers whose buffer is full will drop the event.
// The recover guards against a theoretical send-on-closed-channel
// if Unsubscribe races with Publish under unusual scheduling.
func (b *EventBus) Publish(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subscribers {
		func() {
			defer func() { recover() }()
			select {
			case ch <- event:
			default:
			}
		}()
	}
}

// FormatSSE serializes an event for the SSE text/event-stream protocol.
func FormatSSE(event Event) ([]byte, error) {
	data, err := json.Marshal(event.Data)
	if err != nil {
		return nil, err
	}
	return []byte("event: " + event.Type + "\ndata: " + string(data) + "\n\n"), nil
}
