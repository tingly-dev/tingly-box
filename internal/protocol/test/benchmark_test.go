package test

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"

	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
)

// Benchmark tests
func BenchmarkConvertAnthropicResponseToOpenAI(b *testing.B) {
	anthropicResp := &anthropic.BetaMessage{
		ID:    "msg_benchmark",
		Model: "claude-3-sonnet",
		Role:  "assistant",
		Type:  "message",
		Content: []anthropic.BetaContentBlockUnion{
			{
				Type: "text",
				Text: "This is a benchmark test response.",
			},
			{
				Type:  "tool_use",
				ID:    "tool_benchmark",
				Name:  "benchmark_tool",
				Input: json.RawMessage(`{"param1":"value1","param2":42}`),
			},
		},
		Usage: anthropic.BetaUsage{
			InputTokens:  100,
			OutputTokens: 200,
		},
		StopReason:   "end_turn",
		StopSequence: "",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nonstream.HandleAnthropicBetaToOpenAIResponse(anthropicResp, "claude-3-sonnet")
	}
}

func BenchmarkConvertOpenAIToAnthropicRequest(b *testing.B) {
	req := &openai.ChatCompletionNewParams{
		Model:     openai.ChatModel("gpt-4"),
		Messages:  []openai.ChatCompletionMessageParamUnion{openai.SystemMessage("You are a helpful assistant."), openai.UserMessage("Hello, how are you?")},
		MaxTokens: openai.Opt(int64(100)),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		request.ConvertOpenAIToAnthropicRequest(req, 8192)
	}
}
