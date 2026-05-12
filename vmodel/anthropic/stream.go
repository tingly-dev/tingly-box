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

// DoneEvent signals end of stream with stop reason.
type DoneEvent struct {
	StopReason string
}

// DefaultStream is a stream adapter for batch-only Anthropic models.
// It calls HandleAnthropic, chunks text content via token.SplitIntoChunks,
// and emits typed stream events. Batch-only models should delegate here.
func DefaultStream(vm VirtualModel, req *protocol.AnthropicBetaMessagesRequest, emit func(any)) error {
	resp, err := vm.HandleAnthropic(req)
	if err != nil {
		return err
	}
	emit(StreamStartEvent{MsgID: "msg_virtual", Model: ""})
	for i, blk := range resp.Content {
		if blk.OfText != nil {
			chunks := token.SplitIntoChunks(blk.OfText.Text)
			vmodel.EmitChunks(chunks, vmodel.DefaultStreamChunkDelay, func(_ int, chunk string) {
				emit(TextDeltaEvent{Index: i, Text: chunk})
			})
		} else if blk.OfToolUse != nil {
			inputJSON, _ := json.Marshal(blk.OfToolUse.Input)
			emit(ToolUseEvent{
				Index: i,
				ID:    blk.OfToolUse.ID,
				Name:  blk.OfToolUse.Name,
				Input: inputJSON,
			})
		}
	}
	emit(DoneEvent{StopReason: string(resp.StopReason)})
	return nil
}
