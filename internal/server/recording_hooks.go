package server

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

// streamRecorder couples a ProtocolRecorder with a stream assembler so that
// raw SSE events emitted during protocol conversion are mirrored into both
// the recorder's chunk log and an assembler that synthesises the final
// response body once the stream ends.
//
// It backs the gin-context StreamEventRecorder path used by the
// Responses→Anthropic conversion handlers. Native Anthropic streams instead
// use AttachRecorderHooks, which relies on the assembler that protocol's
// HandleContext now owns.
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

func (sr *streamRecorder) SetupStreamRecorderInContext(c *gin.Context) {
	if sr == nil {
		return
	}
	c.Set(streamRecorderContextKey, sr)
}

// AttachRecorderHooks wires a ProtocolRecorder into a native Anthropic stream
// HandleContext. Raw SSE chunks are mirrored into the recorder's chunk log;
// the assembled final response is produced by the protocol-side assembler
// (HandleContext.streamAssembler) and delivered via the assembled hook;
// completion and error finalise the record.
func AttachRecorderHooks(hc *protocol.HandleContext, recorder *ProtocolRecorder, model string, provider *typ.Provider) {
	if recorder == nil {
		return
	}
	recorder.EnableStreaming()

	hc.WithOnStreamEvent(func(event interface{}) error {
		recorder.RecordStreamChunk(streamEventType(event), event)
		return nil
	})
	hc.WithOnStreamAssembled(func(msg *anthropic.Message) {
		if msg == nil {
			return
		}
		msg.Model = anthropic.Model(model)
		recorder.SetAssembledResponse(msg)
	})
	hc.WithOnStreamComplete(func() {
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
