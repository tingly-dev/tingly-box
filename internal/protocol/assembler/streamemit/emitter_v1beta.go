package streamemit

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// FeedV1Beta routes a v1beta Anthropic stream event through the emitter
// and returns the events to send to the consumer right now.
func (e *StreamEmitter) FeedV1Beta(evt *anthropic.BetaRawMessageStreamEventUnion) ([]BufferedEvent, error) {
	if evt == nil {
		return nil, nil
	}
	if e.version == versionUnset {
		e.version = versionV1Beta
	} else if e.version != versionV1Beta {
		return nil, ErrMixedVersions
	}

	e.msg.RecordV1BetaEvent(evt)

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
		kind, toolID := decideBlockKindV1Beta(evt.ContentBlock)
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
