package events

import (
	"time"
)

// Event represents a generic agent event
type Event struct {
	Type      string                 `json:"type"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
	Raw       string                 `json:"raw,omitempty"`
}

// NewEvent creates a new event with the current timestamp
func NewEvent(eventType string, data map[string]interface{}) Event {
	return Event{
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now(),
	}
}

// NewEventFromRaw creates an event from a raw JSON string
func NewEventFromRaw(raw string, data map[string]interface{}) Event {
	eventType := "unknown"
	if t, ok := data["type"].(string); ok {
		eventType = t
	}

	return Event{
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now(),
		Raw:       raw,
	}
}

// EventHandler processes events
type EventHandler interface {
	HandleEvent(event Event) error
}

// EventHandlerFunc is a function adapter for EventHandler
type EventHandlerFunc func(event Event) error

// HandleEvent implements EventHandler interface
func (f EventHandlerFunc) HandleEvent(event Event) error {
	return f(event)
}

// Listener receives events
type Listener interface {
	OnEvent(event Event)
}

// ListenerFunc is a function adapter for Listener
type ListenerFunc func(event Event)

// OnEvent implements Listener interface
func (f ListenerFunc) OnEvent(event Event) {
	f(event)
}
