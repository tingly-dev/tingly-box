package streamemit

import "github.com/tingly-dev/tingly-box/internal/protocol"

// BufferedEvent is one Anthropic SSE event ready to be sent to the consumer.
//
// It is a type alias of protocol.GuardrailsBufferedEvent so the output of
// this package is byte-compatible with the guardrails rewrite layer's
// buffered events: callers can append slices from both sources and feed
// them into the same emitter without conversion.
type BufferedEvent = protocol.GuardrailsBufferedEvent
