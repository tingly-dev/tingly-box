package stream

import (
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

type testFlusher struct{}

func (testFlusher) Flush() {}

// writeAnthropicSSE drives a converter that emits Anthropic events and writes
// them to the client as SSE, as the Stage adapter's writers do, so tests can
// pin a converter's wire output and usage.
func writeAnthropicSSE(hc *protocol.HandleContext, conv StreamConverter) (*protocol.TokenUsage, error) {
	c := hc.GinContext
	_, err := RunConverter(hc, conv, func(event interface{}) error {
		if e, ok := event.(anthropicStreamEvent); ok {
			sendAnthropicStreamEvent(c, e.eventType, e.data, testFlusher{})
		}
		return nil
	})
	return conv.Usage(), err
}
