package recording

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/assembler"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// streamRecorderContextKey is the gin context key under which the active
// streamRecorder is published for protocol-layer code to feed events into.
const streamRecorderContextKey = "stream_event_recorder"

// StreamRecorder couples a ProtocolRecorder with a stream assembler so that
// raw SSE events emitted during protocol conversion are mirrored into both
// the recorder's chunk log and an assembler that synthesises the final
// response body once the stream ends.
//
// It backs the gin-context StreamEventRecorder path used by the
// Responses→Anthropic conversion handlers. Native Anthropic streams instead
// use AttachRecorderHooks, which relies on the assembler that protocol's
// HandleContext now owns. Exported (moved from unexported streamRecorder)
// because root's protocol_cross.go (Step 7 territory) still constructs one
// directly via NewStreamRecorder and calls its methods.
type StreamRecorder struct {
	recorder        *ProtocolRecorder
	assembler       *assembler.AnthropicStreamAssembler
	inputTokens     int
	outputTokens    int
	cacheReadTokens int
	hasUsage        bool
}

// NewStreamRecorder is the exported constructor for StreamRecorder.
func NewStreamRecorder(recorder *ProtocolRecorder) *StreamRecorder {
	if recorder == nil {
		return nil
	}
	recorder.EnableStreaming()
	return &StreamRecorder{
		recorder:  recorder,
		assembler: assembler.NewAnthropicStreamAssembler(),
	}
}

// Finish carries the converter's final TokenUsage into the assembler so the
// recorded response has the full shape (input/output + cache_read).
// When usage is nil or empty, any in-stream usage harvested via
// RecordRawMapEvent is used as a fallback.
func (sr *StreamRecorder) Finish(model string, usage *protocol.TokenUsage) {
	if sr == nil {
		return
	}
	if (usage == nil || (usage.InputTokens == 0 && usage.OutputTokens == 0)) && sr.hasUsage {
		usage = &protocol.TokenUsage{
			InputTokens:      sr.inputTokens,
			OutputTokens:     sr.outputTokens,
			CacheInputTokens: sr.cacheReadTokens,
		}
	}
	if usage == nil {
		usage = &protocol.TokenUsage{}
	}
	sr.assembler.SetUsageFromTokenUsage(usage)

	if assembled := sr.assembler.Finish(model, usage.InputTokens, usage.OutputTokens); assembled != nil {
		sr.recorder.SetAssembledResponse(assembled)
		return
	}
	if len(sr.recorder.streamChunks) == 0 {
		return
	}
	fallback := baseMessageMap(model, sr.recorder.startTime)
	fallback["stop_reason"] = sr.recorder.c.Query("stop_reason")
	usageMap := map[string]interface{}{
		"input_tokens":  usage.InputTokens,
		"output_tokens": usage.OutputTokens,
	}
	if usage.CacheInputTokens > 0 {
		usageMap["cache_read_input_tokens"] = usage.CacheInputTokens
	}
	fallback["usage"] = usageMap
	sr.recorder.SetAssembledResponse(fallback)
	logrus.Debugf("obs: streamRecorder using fallback response, chunks=%d", len(sr.recorder.streamChunks))
}

func (sr *StreamRecorder) RecordError(err error) {
	if sr == nil {
		return
	}
	sr.recorder.RecordError(err)
}

func (sr *StreamRecorder) RecordResponse(provider *typ.Provider, model string) {
	if sr == nil {
		return
	}
	sr.recorder.RecordResponse(provider, model)
}

// RecordRawMapEvent feeds a generic map-encoded SSE event into both the
// assembler (best-effort) and the recorder's chunk log. Updates the usage
// counters from message_delta events.
func (sr *StreamRecorder) RecordRawMapEvent(eventType string, event map[string]interface{}) {
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
			if v, ok := mapInt(usage, "cache_read_input_tokens"); ok {
				sr.cacheReadTokens = v
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

func (sr *StreamRecorder) SetupStreamRecorderInContext(c *gin.Context) {
	if sr == nil {
		return
	}
	c.Set(streamRecorderContextKey, sr)
}

// AttachRecorderHooks wires a ProtocolRecorder into a native Anthropic stream
// HandleContext. Raw SSE chunks are mirrored into the recorder's chunk log;
// an internal assembler synthesises the final *anthropic.Message once the
// stream completes; completion and error finalise the record.
func AttachRecorderHooks(hc *protocol.HandleContext, recorder *ProtocolRecorder, model string, provider *typ.Provider) {
	if recorder == nil {
		return
	}
	recorder.EnableStreaming()

	asm := assembler.NewAnthropicStreamAssembler()

	hc.WithOnStreamEvent(func(event interface{}) error {
		recorder.RecordStreamChunk(streamEventType(event), event)
		switch evt := event.(type) {
		case *anthropic.MessageStreamEventUnion:
			asm.RecordV1Event(evt)
		case *anthropic.BetaRawMessageStreamEventUnion:
			asm.RecordV1BetaEvent(evt)
		}
		return nil
	})
	hc.WithOnStreamComplete(func() {
		if msg := asm.Finish(model, 0, 0); msg != nil {
			recorder.SetAssembledResponse(msg)
		}
		recorder.RecordResponse(provider, model)
	})
	hc.WithOnStreamError(func(err error) {
		recorder.RecordError(err)
	})
}

// streamEventType extracts the SSE event type from a typed Anthropic stream
// event union, used as a fallback label for the recorder's chunk log.
func streamEventType(event interface{}) string {
	switch evt := event.(type) {
	case *anthropic.MessageStreamEventUnion:
		return evt.Type
	case *anthropic.BetaRawMessageStreamEventUnion:
		return evt.Type
	}
	return ""
}
