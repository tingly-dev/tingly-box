package adaptor

import (
	"encoding/json"
	"testing"
	"tingly-box/pkg/adaptor/request"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
)

// Benchmark tests
func BenchmarkConvertAnthropicResponseToOpenAI(b *testing.B) {
	anthropicResp := &anthropic.Message{
		ID:    "msg_benchmark",
		Model: "claude-3-sonnet",
		Role:  "assistant",
		Type:  "message",
		Content: []anthropic.ContentBlockUnion{
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
		Usage: anthropic.Usage{
			InputTokens:  100,
			OutputTokens: 200,
		},
		StopReason:   "end_turn",
		StopSequence: "",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ConvertAnthropicToOpenAIResponse(anthropicResp, "claude-3-sonnet")
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
