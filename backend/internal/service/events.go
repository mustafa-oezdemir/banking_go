package service

import (
	"sync"

	"github.com/google/uuid"
)

// EventHub fans out lightweight payment notifications to authenticated SSE
// clients. Durable event history remains in PostgreSQL audit_events.
type EventHub struct {
	mu          sync.RWMutex
	subscribers map[uuid.UUID]map[chan struct{}]struct{}
}

func NewEventHub() *EventHub {
	return &EventHub{subscribers: make(map[uuid.UUID]map[chan struct{}]struct{})}
}

func (h *EventHub) Subscribe(ownerID uuid.UUID) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	if h.subscribers[ownerID] == nil {
		h.subscribers[ownerID] = make(map[chan struct{}]struct{})
	}
	h.subscribers[ownerID][ch] = struct{}{}
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		delete(h.subscribers[ownerID], ch)
		if len(h.subscribers[ownerID]) == 0 {
			delete(h.subscribers, ownerID)
		}
		h.mu.Unlock()
	}
}

func (h *EventHub) Publish(ownerID uuid.UUID) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subscribers[ownerID] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
