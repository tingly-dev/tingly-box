package virtualmodel

import (
	"encoding/json"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
)

// DefaultAnthropicStream is a stream adapter for batch-only Anthropic models.
// It calls HandleAnthropic, chunks text content via token.SplitIntoChunks,
// and emits typed stream events. Batch-only models should delegate here.
func DefaultAnthropicStream(vm AnthropicVirtualModel, req *protocol.AnthropicBetaMessagesRequest, emit func(any)) error {
	resp, err := vm.HandleAnthropic(req)
	if err != nil {
		return err
	}
	emit(AnthropicStreamStartEvent{MsgID: "msg_virtual", Model: ""})
	for i, blk := range resp.Content {
		if blk.OfText != nil {
			for _, chunk := range token.SplitIntoChunks(blk.OfText.Text) {
				time.Sleep(50 * time.Millisecond)
				emit(AnthropicTextDeltaEvent{Index: i, Text: chunk})
			}
		} else if blk.OfToolUse != nil {
			inputJSON, _ := json.Marshal(blk.OfToolUse.Input)
			emit(AnthropicToolUseEvent{
				Index: i,
				ID:    blk.OfToolUse.ID,
				Name:  blk.OfToolUse.Name,
				Input: inputJSON,
			})
		}
	}
	emit(AnthropicDoneEvent{StopReason: string(resp.StopReason)})
	return nil
}

// DefaultOpenAIChatStream is a stream adapter for batch-only OpenAI Chat models.
// It calls HandleOpenAIChat, chunks text content via token.SplitIntoChunks,
// and emits typed stream events. Batch-only models should delegate here.
func DefaultOpenAIChatStream(vm OpenAIChatVirtualModel, req *protocol.OpenAIChatCompletionRequest, emit func(any)) error {
	resp, err := vm.HandleOpenAIChat(req)
	if err != nil {
		return err
	}
	chunks := token.SplitIntoChunks(resp.Content)
	for i, chunk := range chunks {
		time.Sleep(50 * time.Millisecond)
		emit(OpenAIChatDeltaEvent{Index: i, Content: chunk})
	}
	for i, tc := range resp.ToolCalls {
		emit(OpenAIChatToolEvent{Index: i, ToolCall: tc})
	}
	emit(OpenAIChatDoneEvent{FinishReason: resp.FinishReason})
	return nil
}
