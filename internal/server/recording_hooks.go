package server

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol/assembler"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// streamRecorderContextKey is the gin context key under which the active
// streamRecorder is published for protocol-layer code to feed events into.
const streamRecorderContextKey = "stream_event_recorder"

// streamRecorder couples a ProtocolRecorder with a stream assembler so that
// raw SSE events are mirrored into both the recorder's chunk log and an
// assembler that synthesises the final response body once the stream ends.
type streamRecorder struct {
	recorder     *ProtocolRecorder
	assembler    *assembler.AnthropicStreamAssembler
	inputTokens  int
	outputTokens int
	hasUsage     bool
}

func newStreamRecorder(recorder *ProtocolRecorder) *streamRecorder {
	if recorder == nil {
		return nil
	}
	recorder.EnableStreaming()
	return &streamRecorder{
		recorder:  recorder,
		assembler: assembler.NewAnthropicStreamAssembler(),
	}
}

func (sr *streamRecorder) RecordV1Event(event *anthropic.MessageStreamEventUnion) {
	if sr == nil {
		return
	}
	sr.recorder.RecordStreamChunk(event.Type, event)
	sr.assembler.RecordV1Event(event)
}

func (sr *streamRecorder) RecordV1BetaEvent(event *anthropic.BetaRawMessageStreamEventUnion) {
	if sr == nil {
		return
	}
	sr.recorder.RecordStreamChunk(event.Type, event)
	sr.assembler.RecordV1BetaEvent(event)
}

func (sr *streamRecorder) Finish(model string, inputTokens, outputTokens int) {
	if sr == nil {
		return
	}
	if inputTokens == 0 && outputTokens == 0 && sr.hasUsage {
		inputTokens = sr.inputTokens
		outputTokens = sr.outputTokens
	}
	assembled := sr.assembler.Finish(model, inputTokens, outputTokens)
	if assembled != nil {
		sr.recorder.SetAssembledResponse(assembled)
		return
	}
	if len(sr.recorder.streamChunks) > 0 {
		fallback := baseMessageMap(model, sr.recorder.startTime)
		fallback["stop_reason"] = sr.recorder.c.Query("stop_reason")
		fallback["usage"] = map[string]interface{}{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		}
		sr.recorder.SetAssembledResponse(fallback)
		logrus.Debugf("obs: streamRecorder using fallback response, chunks=%d", len(sr.recorder.streamChunks))
	}
}

func (sr *streamRecorder) RecordError(err error) {
	if sr == nil {
		return
	}
	sr.recorder.RecordError(err)
}

func (sr *streamRecorder) RecordResponse(provider *typ.Provider, model string) {
	if sr == nil {
		return
	}
	sr.recorder.RecordResponse(provider, model)
}

// RecordRawMapEvent feeds a generic map-encoded SSE event into both the
// assembler (best-effort) and the recorder's chunk log. Updates the usage
// counters from message_delta events.
func (sr *streamRecorder) RecordRawMapEvent(eventType string, event map[string]interface{}) {
	if sr == nil {
		return
	}
	if data, err := json.Marshal(event); err == nil {
		var betaEvent anthropic.BetaRawMessageStreamEventUnion
		if err := json.Unmarshal(data, &betaEvent); err == nil {
			betaEvent.Type = eventType
			sr.assembler.RecordV1BetaEvent(&betaEvent)
		}
	}
	sr.recorder.RecordStreamChunk(eventType, event)

	if eventType == "message_delta" {
		if usage, ok := event["usage"].(map[string]interface{}); ok {
			if v, ok := mapInt(usage, "input_tokens"); ok {
				sr.inputTokens = v
			}
			if v, ok := mapInt(usage, "output_tokens"); ok {
				sr.outputTokens = v
			}
			sr.hasUsage = true
		}
	}
}

// mapInt reads a JSON-decoded numeric value (float64 or int64) from m.
func mapInt(m map[string]interface{}, key string) (int, bool) {
	switch v := m[key].(type) {
	case float64:
		return int(v), true
	case int64:
		return int(v), true
	}
	return 0, false
}

func (sr *streamRecorder) StreamEventRecorder() interface{} {
	if sr == nil {
		return nil
	}
	return sr
}

func (sr *streamRecorder) SetupStreamRecorderInContext(c *gin.Context) {
	if sr == nil {
		return
	}
	c.Set(streamRecorderContextKey, sr)
}

func (sr *streamRecorder) updateUsageFromTyped(in, out int64) {
	if in > 0 {
		sr.inputTokens = int(in)
		sr.hasUsage = true
	}
	if out > 0 {
		sr.outputTokens = int(out)
		sr.hasUsage = true
	}
}

// NewRecorderHooks builds streaming hooks bound to a recorder. On completion
// the assembled response is finalised and RecordResponse is invoked with the
// provided model/provider — callers that want to defer RecordResponse should
// pass an empty model and call RecordResponse themselves.
func NewRecorderHooks(recorder *ProtocolRecorder, model string, provider *typ.Provider) (onStreamEvent func(event interface{}) error, onStreamComplete func(), onStreamError func(err error)) {
	if recorder == nil {
		return nil, nil, nil
	}

	streamRec := newStreamRecorder(recorder)

	onStreamEvent = func(event interface{}) error {
		if streamRec == nil {
			return nil
		}
		switch evt := event.(type) {
		case *anthropic.MessageStreamEventUnion:
			streamRec.RecordV1Event(evt)
			streamRec.updateUsageFromTyped(evt.Usage.InputTokens, evt.Usage.OutputTokens)
		case *anthropic.BetaRawMessageStreamEventUnion:
			streamRec.RecordV1BetaEvent(evt)
			streamRec.updateUsageFromTyped(evt.Usage.InputTokens, evt.Usage.OutputTokens)
		case map[string]interface{}:
			if eventType, ok := evt["type"].(string); ok {
				streamRec.RecordRawMapEvent(eventType, evt)
			}
		}
		return nil
	}

	onStreamComplete = func() {
		if streamRec == nil {
			return
		}
		streamRec.Finish(model, streamRec.inputTokens, streamRec.outputTokens)
		streamRec.RecordResponse(provider, model)
	}

	onStreamError = func(err error) {
		if streamRec == nil {
			return
		}
		streamRec.RecordError(err)
	}

	return onStreamEvent, onStreamComplete, onStreamError
}
