package streamemit

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// FeedV1 routes a v1 Anthropic stream event through the emitter and
// returns the (possibly empty) events to send to the consumer right now.
func (e *StreamEmitter) FeedV1(evt *anthropic.MessageStreamEventUnion) ([]BufferedEvent, error) {
	if evt == nil {
		return nil, nil
	}
	if e.version == versionUnset {
		e.version = versionV1
	} else if e.version != versionV1 {
		return nil, ErrMixedVersions
	}

	// Always feed the inner assembler so MessageAssembler() and Finish()
	// stay accurate regardless of buffering decisions.
	e.msg.RecordV1Event(evt)

	payload, err := payloadFromRaw(strings.Clone(evt.RawJSON()), evt.Type)
	if err != nil {
		return nil, err
	}
	buf := BufferedEvent{EventType: evt.Type, Payload: payload}
	index := int(evt.Index)

	switch evt.Type {
	case "message_start", "message_delta", "message_stop":
		return []BufferedEvent{buf}, nil

	case "content_block_start":
		kind, toolID := decideBlockKindV1(evt.ContentBlock)
		e.kinds[index] = kind
		if kind == BlockKindToolUse {
			e.toolIDs[index] = toolID
		}
		if e.shouldBuffer(kind) {
			e.openBuffer(index, toolID, buf)
			return nil, nil
		}
		return []BufferedEvent{buf}, nil

	case "content_block_delta":
		if e.isBuffered(index) {
			e.toolBufs[index].append(buf)
			return nil, nil
		}
		return []BufferedEvent{buf}, nil

	case "content_block_stop":
		if e.isBuffered(index) {
			e.toolBufs[index].append(buf)
			return e.flushToolBuffer(index)
		}
		return []BufferedEvent{buf}, nil

	default:
		return []BufferedEvent{buf}, nil
	}
}

// shouldBuffer reports whether new blocks of the given kind should be
// held under EmitOnComplete given the emitter's policy.
func (e *StreamEmitter) shouldBuffer(kind BlockKind) bool {
	switch kind {
	case BlockKindText:
		return e.cfg.TextPolicy == EmitOnComplete
	case BlockKindThinking:
		return e.cfg.ThinkingPolicy == EmitOnComplete
	case BlockKindToolUse:
		return e.cfg.ToolPolicy == EmitOnComplete
	default:
		return false
	}
}

// openBuffer creates a new toolBlockBuffer for the given index and seeds
// it with the start event. We reuse the tool buffer machinery for any
// kind that is configured to buffer (text/thinking/tool_use).
func (e *StreamEmitter) openBuffer(index int, toolID string, start BufferedEvent) {
	buf := newToolBlockBuffer(index, toolID)
	buf.append(start)
	e.toolBufs[index] = buf
}

// isBuffered reports whether the given content block index currently has
// a live buffer (i.e. its start event was held and the block hasn't been
// flushed yet).
func (e *StreamEmitter) isBuffered(index int) bool {
	_, ok := e.toolBufs[index]
	return ok
}
