package managedagent

import (
	"context"
	"sync"
)

// EventBus decorates an EventStore so every appended event is also handed
// to in-process subscribers, after it is durably stored. It is the one
// seam observers (the IM bridge, a future SSE fan-out) attach to; neither
// the Service nor the Launcher knows they exist.
//
// Subscribers run synchronously on the appending goroutine and must not
// block: anything slow (a chat prompt) is spawned by the subscriber.
type EventBus struct {
	store EventStore
	mu    sync.RWMutex
	subs  []func(Event)
}

var _ EventStore = (*EventBus)(nil)

// NewEventBus wraps a store.
func NewEventBus(store EventStore) *EventBus {
	return &EventBus{store: store}
}

// Subscribe registers an observer for every event appended from now on.
func (b *EventBus) Subscribe(fn func(Event)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs = append(b.subs, fn)
}

func (b *EventBus) AppendEvent(ctx context.Context, e *Event) error {
	if err := b.store.AppendEvent(ctx, e); err != nil {
		return err
	}
	b.mu.RLock()
	subs := append([]func(Event){}, b.subs...)
	b.mu.RUnlock()
	for _, fn := range subs {
		fn(*e)
	}
	return nil
}

func (b *EventBus) ListEvents(ctx context.Context, sessionID string, after int64, limit int) ([]Event, error) {
	return b.store.ListEvents(ctx, sessionID, after, limit)
}
