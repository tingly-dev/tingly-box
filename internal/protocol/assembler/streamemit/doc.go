// Package streamemit provides a decoupled emission layer on top of the
// Anthropic stream assemblers in internal/protocol/assembler.
//
// A StreamEmitter routes incoming Anthropic stream events to per-kind
// sub-buffers and applies a configurable EmissionPolicy per kind. The
// supported policies are EmitImmediate (the default — events flow out
// 1:1 as they arrive) and EmitOnComplete (events for a content block
// are buffered from content_block_start through content_block_stop and
// flushed as a single ordered slice on stop).
//
// The primary use case is "hold tool_use blocks until the assembler
// has the complete tool call" while still streaming text and thinking
// to the consumer live. This mirrors the buffer/decision pattern used
// by internal/guardrails/mutate for credential masking, but generalizes
// it so non-guardrails callers can compose the same shape.
//
// The emitter owns its inner *assembler.AnthropicStreamAssembler. It
// always feeds every event to the inner assembler, regardless of any
// buffering, so MessageAssembler() and Finish() always reflect the
// full stream so far. Callers that today drive their own assembler
// in parallel (e.g. internal/server/scenario_recording.go) should
// pick one or the other to avoid double-feeding.
//
// Scope: this package supports Anthropic v1 (MessageStreamEventUnion)
// and v1beta (BetaRawMessageStreamEventUnion). OpenAI Chat and OpenAI
// Responses streams are out of scope.
package streamemit
