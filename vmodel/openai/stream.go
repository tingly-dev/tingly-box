package openai

import (
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/vmodel"
)

// DeltaEvent carries a text content chunk.
type DeltaEvent struct {
	Index   int
	Content string
}

// ToolEvent carries a tool call.
type ToolEvent struct {
	Index    int
	ToolCall VToolCall
}

// UsageEvent carries explicit token usage that the model wants the wire
// stream to advertise (typically just before DoneEvent).
type UsageEvent struct {
	Usage vmodel.MockUsage
}

// DoneEvent signals end of stream with finish reason.
type DoneEvent struct {
	FinishReason string
}

// DefaultStream is a stream adapter for batch-only OpenAI Chat models.
// It calls HandleOpenAIChat, chunks text content, and emits typed stream events.
// Batch-only models should delegate here.
func DefaultStream(vm VirtualModel, req *protocol.OpenAIChatCompletionRequest, emit func(any)) error {
	resp, err := vm.HandleOpenAIChat(req)
	if err != nil {
		return err
	}
	chunks := token.SplitIntoChunks(resp.Content)
	vmodel.EmitChunks(chunks, vmodel.DefaultStreamChunkDelay, func(i int, chunk string) {
		emit(DeltaEvent{Index: i, Content: chunk})
	})
	for i, tc := range resp.ToolCalls {
		emit(ToolEvent{Index: i, ToolCall: tc})
	}
	emit(DoneEvent{FinishReason: resp.FinishReason})
	return nil
}
