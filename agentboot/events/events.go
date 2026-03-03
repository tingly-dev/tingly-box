package events

import (
	"encoding/json"
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
func NewEventFromMap(data map[string]interface{}) Event {
	eventType := "unknown"
	if t, ok := data["type"].(string); ok {
		eventType = t
	}

	var raw string
	if data != nil {
		bs, _ := json.Marshal(data)
		raw = string(bs)
	}

	return Event{
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now(),
		Raw:       raw,
	}
}

// NewEventFromRaw creates an event from a raw JSON string
func NewEventFromRaw(raw string) Event {
	data := map[string]any{}
	err := json.Unmarshal([]byte(raw), &data)
	if err != nil {
		return Event{}
	}

	eventType := "unknown"
	if t, ok := data["type"].(string); ok {
		eventType = t
	}

	if data != nil && raw == "" {
		bs, _ := json.Marshal(data)
		raw = string(bs)
	}

	return Event{
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now(),
		Raw:       raw,
	}
}
