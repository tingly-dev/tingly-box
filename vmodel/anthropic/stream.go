package anthropic

import (
	"encoding/json"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/vmodel"
)

// StreamStartEvent signals the start of a message stream.
type StreamStartEvent struct {
	MsgID string
	Model string
}

// TextDeltaEvent carries a text content chunk.
type TextDeltaEvent struct {
	Index int
	Text  string
}

// ToolUseEvent carries a complete tool_use block.
type ToolUseEvent struct {
	Index int
	ID    string
	Name  string
	Input json.RawMessage
}

// UsageEvent carries explicit token usage that the model wants the wire
// stream to advertise (typically just before DoneEvent — emitted by
// virtualserver inside message_delta.usage).
type UsageEvent struct {
	Usage vmodel.MockUsage
}

// DoneEvent signals end of stream with stop reason.
type DoneEvent struct {
	StopReason string
}

// DefaultStream is a stream adapter for batch-only Anthropic models.
// It calls HandleAnthropic, chunks text content via token.SplitIntoChunks,
// and emits typed stream events. Batch-only models should delegate here.
//
// If the model implements vmodel.ErrorInjectingModel with a mid-stream
// injection, this loop stops after the configured event count and returns
// without emitting DoneEvent. The virtualserver handler then applies the
// configured break mode (TCP close or SSE error event).
func DefaultStream(vm VirtualModel, req *protocol.AnthropicBetaMessagesRequest, emit func(any)) error {
	resp, err := vm.HandleAnthropic(req)
	if err != nil {
		return err
	}
	gate := vmodel.NewEmitGate(vmodel.MidStreamCutoff(vm))
	if !gate.Allow() {
		return nil
	}
	emit(StreamStartEvent{MsgID: "msg_virtual", Model: ""})
	for i, blk := range resp.Content {
		if blk.OfText != nil {
			chunks := token.SplitIntoChunks(blk.OfText.Text)
			if vmodel.EmitChunksGated(chunks, vmodel.DefaultStreamChunkDelay, gate, func(_ int, chunk string) {
				emit(TextDeltaEvent{Index: i, Text: chunk})
			}) {
				return nil
			}
		} else if blk.OfToolUse != nil {
			if !gate.Allow() {
				return nil
			}
			inputJSON, _ := json.Marshal(blk.OfToolUse.Input)
			emit(ToolUseEvent{
				Index: i,
				ID:    blk.OfToolUse.ID,
				Name:  blk.OfToolUse.Name,
				Input: inputJSON,
			})
		}
	}
	if !gate.Allow() {
		return nil
	}
	emit(DoneEvent{StopReason: string(resp.StopReason)})
	return nil
}
