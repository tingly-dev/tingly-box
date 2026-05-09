package streamemit

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol/assembler"
)

// ErrMixedVersions is returned by Feed when an emitter that has already
// processed v1 events receives a v1beta event (or vice versa).
var ErrMixedVersions = errors.New("streamemit: cannot mix v1 and v1beta events on a single emitter")

// emitterVersion pins an emitter to a single Anthropic event union type
// after the first Feed call.
type emitterVersion uint8

const (
	versionUnset emitterVersion = iota
	versionV1
	versionV1Beta
)

// StreamEmitter routes Anthropic stream events through a configurable
// emission policy. See package doc for usage.
type StreamEmitter struct {
	cfg Config

	// msg is the inner state machine used to produce the final
	// *anthropic.Message. It is fed every event regardless of buffering.
	msg *assembler.AnthropicStreamAssembler

	// kinds tracks BlockKind per content block index, learned at
	// content_block_start, used to route subsequent deltas and stops.
	kinds map[int]BlockKind

	// toolIDs is the tool_use id per content block index, populated when
	// a tool_use block opens.
	toolIDs map[int]string

	// toolBufs holds per-block buffers for tool_use blocks under the
	// EmitOnComplete policy. A block index has an entry here only while
	// it is being buffered; the entry is removed on flush/drain.
	toolBufs map[int]*toolBlockBuffer

	version emitterVersion
}

// New constructs a StreamEmitter with the given configuration.
func New(cfg Config) *StreamEmitter {
	return &StreamEmitter{
		cfg:      cfg,
		msg:      assembler.NewAnthropicStreamAssembler(),
		kinds:    make(map[int]BlockKind),
		toolIDs:  make(map[int]string),
		toolBufs: make(map[int]*toolBlockBuffer),
	}
}

// MessageAssembler returns the inner *AnthropicStreamAssembler so callers
// can inspect message-side accumulation independently of tool buffering.
// Callers should treat it as read-only — feeding events into it directly
// will desync it from the emitter's routing state.
func (e *StreamEmitter) MessageAssembler() *assembler.AnthropicStreamAssembler {
	return e.msg
}

// ToolBuffer returns a copy of the events currently buffered for the given
// content block index. The bool is false when no buffer exists for that
// index (either it was never tool_use, it was emitted immediately, or it
// has already been flushed).
func (e *StreamEmitter) ToolBuffer(index int) ([]BufferedEvent, bool) {
	buf, ok := e.toolBufs[index]
	if !ok {
		return nil, false
	}
	return buf.snapshot(), true
}

// Drain flushes every still-open tool buffer, returning the buffered events
// in ascending block-index order, and clears the buffers. Useful at end of
// stream or on error when the caller wants whatever has been accumulated.
//
// Drain does NOT call OnToolBlockComplete — it is a salvage path, not a
// completion signal.
func (e *StreamEmitter) Drain() []BufferedEvent {
	if len(e.toolBufs) == 0 {
		return nil
	}
	indices := make([]int, 0, len(e.toolBufs))
	for idx := range e.toolBufs {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	var out []BufferedEvent
	for _, idx := range indices {
		out = append(out, e.toolBufs[idx].drain()...)
		delete(e.toolBufs, idx)
	}
	return out
}

// Finish drains any remaining tool buffers and returns the assembled
// *anthropic.Message produced by the inner assembler.
//
// model, inputTokens, outputTokens are forwarded to
// AnthropicStreamAssembler.Finish.
func (e *StreamEmitter) Finish(model string, inputTokens, outputTokens int) ([]BufferedEvent, *anthropic.Message) {
	pending := e.Drain()
	return pending, e.msg.Finish(model, inputTokens, outputTokens)
}

// Feed accepts either a *anthropic.MessageStreamEventUnion (v1) or a
// *anthropic.BetaRawMessageStreamEventUnion (v1beta) and returns the
// events the caller should send to the consumer right now.
//
// An emitter is pinned to its first version; mixing versions returns
// ErrMixedVersions.
func (e *StreamEmitter) Feed(event interface{}) ([]BufferedEvent, error) {
	switch evt := event.(type) {
	case *anthropic.MessageStreamEventUnion:
		return e.FeedV1(evt)
	case *anthropic.BetaRawMessageStreamEventUnion:
		return e.FeedV1Beta(evt)
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("streamemit: unsupported event type %T", event)
	}
}

// payloadFromRaw unmarshals an Anthropic event's RawJSON into a payload
// map, ensuring the "type" field is set to eventType. The returned map
// matches the shape that sendAnthropicStreamEvent already consumes and
// that GuardrailsBufferedEvent.Payload uses, so emitter output can be
// passed to either consumer without translation.
func payloadFromRaw(rawJSON string, eventType string) (map[string]interface{}, error) {
	payload := map[string]interface{}{}
	if rawJSON != "" {
		if err := json.Unmarshal([]byte(rawJSON), &payload); err != nil {
			return nil, err
		}
	}
	if eventType != "" {
		payload["type"] = eventType
	}
	return payload, nil
}

// flushToolBuffer drains a tool_use buffer at the given index, runs the
// OnToolBlockComplete hook if configured, and returns the events that
// should be released to the caller.
func (e *StreamEmitter) flushToolBuffer(index int) ([]BufferedEvent, error) {
	buf, ok := e.toolBufs[index]
	if !ok {
		return nil, nil
	}
	delete(e.toolBufs, index)

	buffered := buf.drain()
	if e.cfg.OnToolBlockComplete == nil {
		return buffered, nil
	}

	decision, err := e.cfg.OnToolBlockComplete(buf.toolID, buf.index, buffered)
	if err != nil {
		return nil, err
	}
	if decision == nil {
		return buffered, nil
	}
	if decision.Drop {
		return nil, nil
	}
	if decision.Replace != nil {
		return decision.Replace, nil
	}
	return buffered, nil
}
