package stage

import (
	"context"

	protocol "github.com/tingly-dev/tingly-box/ai"
)

// Call is one invocation of an Endpoint in that endpoint's native protocol.
// Request remains protocol-native; a Bridge is responsible for changing its
// concrete type before it crosses a protocol boundary.
type Call struct {
	Request  any
	Metadata CallMetadata
	State    ProtocolState
}

// ProtocolState carries typed, request-derived facts that a later endpoint or
// stage still needs after the request changes protocol. It deliberately avoids
// an open-ended property bag: every carried value needs an explicit contract.
//
// Values are per-call and may be mutated by inner request transforms. A Bridge
// must never retain them on the shared Bridge instance.
type ProtocolState struct {
	// OpenAIChat is populated when a request is converted to OpenAI Chat. It
	// preserves reasoning/thinking facts used by provider-specific transforms.
	OpenAIChat *protocol.OpenAIConfig
}

// CallMetadata carries the small set of attempt identity fields that every
// stage may need. It is immutable by convention: a stage should copy Call
// before changing metadata for an inner invocation.
type CallMetadata struct {
	RequestID string
	// Attempt is zero for the first provider attempt and increments for retries.
	Attempt int
}

// Response is the complete result returned by an Endpoint. Value is expressed
// in the endpoint's native protocol. The remaining fields are protocol-neutral
// facts that outer stages and the eventual failover adapter must preserve.
type Response struct {
	Value                any
	Usage                *protocol.TokenUsage
	Model                string
	SideEffectsCommitted bool
}

// Event is one native-protocol streaming event.
type Event struct {
	Value any
}

// StreamResult is the latest protocol-neutral summary of an EventStream. It is
// valid before completion, but usage and model data may only become final after
// Next returns io.EOF.
type StreamResult struct {
	Usage                *protocol.TokenUsage
	Model                string
	SideEffectsCommitted bool
}

// EventStream is a pull-based stream in one concrete protocol.
//
// Next returns io.EOF on normal completion and must honor ctx cancellation.
// The caller must call Close exactly once for every successfully returned
// EventStream.
// Result returns the latest terminal summary and must not advance the stream.
type EventStream interface {
	Next(ctx context.Context) (Event, error)
	Close() error
	Result() StreamResult
}

// Endpoint is a complete non-streaming and streaming implementation of one
// concrete protocol. It does not own HTTP parsing, response headers, or SSE
// framing.
type Endpoint interface {
	Protocol() protocol.APIType
	Complete(ctx context.Context, call Call) (*Response, error)
	Stream(ctx context.Context, call Call) (EventStream, error)
}

// Stage is a named full-duplex wrapper implemented in one concrete protocol.
// Wrap must not execute next. Compose validates the protocol reported by both
// the Stage and the wrapped Endpoint.
type Stage interface {
	Name() string
	Protocol() protocol.APIType
	Wrap(next Endpoint) Endpoint
}
