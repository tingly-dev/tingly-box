package vmodel


// ErrorStage selects when in the request lifecycle a mock model's error
// injection fires.
type ErrorStage int

const (
	// ErrorStagePreContent is a failure before any response body is written:
	// the handler returns the configured HTTP status with an error JSON envelope
	// and never invokes the model's Handle* methods past that point. This is
	// the case that priority-routing failover MUST retry — the gate stays
	// buffered, the orchestrator sees a retryable status and discards.
	ErrorStagePreContent ErrorStage = iota

	// ErrorStageMidStream is a failure after the handler has already started
	// streaming. The model emits AfterEvents real stream events, then the
	// handler applies MidStreamMode: it either hijacks and closes the TCP
	// connection (truncated stream from the client's POV) or emits a single
	// SSE error event before stopping. This is the case that priority-routing
	// failover MUST NOT retry — the gate has committed, bytes left the process.
	ErrorStageMidStream
)

// MidStreamMode selects how a mid-stream failure terminates the stream after
// AfterEvents events have been emitted to the wire.
type MidStreamMode int

const (
	// MidStreamModeConnectionClose hijacks the underlying TCP connection and
	// closes it. The client sees an EOF / abrupt disconnect mid-stream —
	// exactly the shape an unstable upstream produces when it dies during a
	// long response.
	MidStreamModeConnectionClose MidStreamMode = iota

	// MidStreamModeErrorEvent emits one final protocol-specific error event
	// (SSE "event: error" frame) before returning. The stream is well-formed
	// up to that point; the client sees an in-band error.
	MidStreamModeErrorEvent
)

// ErrorInjection describes a synthetic failure that a mock virtual model
// should simulate. Attach one to MockScenario.Error (or MockModelConfig.Error)
// and the virtualserver handler honors it without any real upstream
// involvement.
//
// Two stages are supported, and they exercise different gateway paths:
//
//   - PreContent: HTTP status + error body, no streaming started. The
//     gateway's firstChunkGate stays buffered → failover retries.
//   - MidStream: handler writes a 200 + Content-Type + AfterEvents real
//     events, then either closes the TCP connection or emits a final SSE
//     error event. firstChunkGate is already committed → failover MUST NOT
//     retry.
type ErrorInjection struct {
	Stage ErrorStage

	// PreContent fields: HTTP status (defaults to 500 if zero) and an
	// optional message that the handler renders into the protocol-specific
	// error envelope (OpenAI: error.message; Anthropic: error.error.message).
	// Type defaults to "api_error" if empty.
	Status  int
	Message string
	Type    string

	// MidStream fields: number of stream events to emit before tripping
	// (default 1; values <= 0 are treated as 1), and how to terminate.
	AfterEvents   int
	MidStreamMode MidStreamMode
}

// ExtractErrorInjection reports the model's error injection configuration,
// or nil if the model does not implement ErrorInjectingModel or has none set.
func ExtractErrorInjection(vm any) *ErrorInjection {
	if ei, ok := vm.(ErrorInjectingModel); ok {
		return ei.ErrorInjection()
	}
	return nil
}

// ErrorInjectingModel is the optional interface a mock virtual model
// implements to declare a synthetic failure. The virtualserver handler
// type-asserts to this interface before dispatching the request.
type ErrorInjectingModel interface {
	ErrorInjection() *ErrorInjection
}

// EmitGate is a counting gate used by mock streams to honor mid-stream
// injection. After Allow() has returned true `cutoff` times, all subsequent
// Allow() calls return false. Callers should bail out of the stream loop the
// first time Allow() returns false to avoid emitting events past the cutoff.
//
// A cutoff of <= 0 disables the gate (Allow() always returns true).
type EmitGate struct {
	cutoff int
	n      int
}

// NewEmitGate constructs a gate. Pass the return value of MidStreamCutoff
// (or -1 to disable).
func NewEmitGate(cutoff int) *EmitGate {
	return &EmitGate{cutoff: cutoff}
}

// Allow reports whether the next event should be emitted, incrementing the
// internal counter on success.
func (g *EmitGate) Allow() bool {
	if g.cutoff > 0 && g.n >= g.cutoff {
		return false
	}
	g.n++
	return true
}

// Tripped reports whether the gate has refused at least one event (or would
// refuse the next one). Use after a stream loop completes to decide whether
// to emit terminal events (DoneEvent / message_stop).
func (g *EmitGate) Tripped() bool {
	return g.cutoff > 0 && g.n >= g.cutoff
}

