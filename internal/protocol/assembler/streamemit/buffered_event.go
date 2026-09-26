package streamemit

// BufferedEvent is one Anthropic SSE event ready to be sent to the consumer.
type BufferedEvent struct {
	EventType string
	Payload   map[string]interface{}
}
