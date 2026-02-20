package events

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// EventBus handles event pub/sub
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string][]EventHandler
	handlers    map[string]Listener
}

// NewEventBus creates a new event bus
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string][]EventHandler),
		handlers:    make(map[string]Listener),
	}
}

// Subscribe subscribes a handler to events of a specific type
// Returns a subscription ID that can be used to unsubscribe
func (eb *EventBus) Subscribe(eventType string, handler EventHandler) string {
	subID := fmt.Sprintf("%s-%d", eventType, time.Now().UnixNano())

	eb.mu.Lock()
	defer eb.mu.Unlock()

	eb.subscribers[eventType] = append(eb.subscribers[eventType], handler)

	return subID
}

// SubscribeAll subscribes a handler to all event types
func (eb *EventBus) SubscribeAll(handler EventHandler) string {
	subID := fmt.Sprintf("all-%d", time.Now().UnixNano())

	eb.mu.Lock()
	defer eb.mu.Unlock()

	eb.subscribers["*"] = append(eb.subscribers["*"], handler)

	return subID
}

// Unsubscribe removes a subscriber
// Note: Current implementation doesn't support individual unsubscribe
// This is a simplified version
func (eb *EventBus) Unsubscribe(subID string) {
	// TODO: Implement individual unsubscribe tracking
}

// Publish publishes an event to all subscribers
func (eb *EventBus) Publish(event Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	// Publish to type-specific subscribers
	if handlers, exists := eb.subscribers[event.Type]; exists {
		for _, handler := range handlers {
			go func(h EventHandler) {
				_ = h.HandleEvent(event)
			}(handler)
		}
	}

	// Publish to wildcard subscribers
	if handlers, exists := eb.subscribers["*"]; exists {
		for _, handler := range handlers {
			go func(h EventHandler) {
				_ = h.HandleEvent(event)
			}(handler)
		}
	}
}

// PublishSync publishes an event synchronously
func (eb *EventBus) PublishSync(event Event) error {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	var lastErr error

	// Publish to type-specific subscribers
	if handlers, exists := eb.subscribers[event.Type]; exists {
		for _, handler := range handlers {
			if err := handler.HandleEvent(event); err != nil {
				lastErr = err
			}
		}
	}

	// Publish to wildcard subscribers
	if handlers, exists := eb.subscribers["*"]; exists {
		for _, handler := range handlers {
			if err := handler.HandleEvent(event); err != nil {
				lastErr = err
			}
		}
	}

	return lastErr
}

// RegisterListener registers a listener for a specific event type
func (eb *EventBus) RegisterListener(eventType string, listener Listener) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	key := fmt.Sprintf("listener-%s", eventType)
	eb.handlers[key] = listener
}

// PublishToListener publishes an event to a registered listener
func (eb *EventBus) PublishToListener(eventType string, event Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	key := fmt.Sprintf("listener-%s", eventType)
	if listener, exists := eb.handlers[key]; exists {
		listener.OnEvent(event)
	}
}

// StartBackgroundProcessor starts a background goroutine that processes events
func (eb *EventBus) StartBackgroundProcessor(ctx context.Context, eventChan <-chan Event) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-eventChan:
				if !ok {
					return
				}
				eb.Publish(event)
			}
		}
	}()
}
