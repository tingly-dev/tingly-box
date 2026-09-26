package recording

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/assembler"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

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
