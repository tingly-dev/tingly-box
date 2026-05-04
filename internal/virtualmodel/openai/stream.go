package openai

import (
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
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

// DoneEvent signals end of stream with finish reason.
type DoneEvent struct {
	FinishReason string
}

// DefaultStream is a stream adapter for batch-only OpenAI Chat models.
// It calls HandleOpenAIChat, chunks text content, and emits typed stream events.
func DefaultStream(vm VirtualModel, req *protocol.OpenAIChatCompletionRequest, emit func(any)) error {
	resp, err := vm.HandleOpenAIChat(req)
	if err != nil {
		return err
	}
	chunks := token.SplitIntoChunks(resp.Content)
	for i, chunk := range chunks {
		time.Sleep(50 * time.Millisecond)
		emit(DeltaEvent{Index: i, Content: chunk})
	}
	for i, tc := range resp.ToolCalls {
		emit(ToolEvent{Index: i, ToolCall: tc})
	}
	emit(DoneEvent{FinishReason: resp.FinishReason})
	return nil
}
