package token

import (
	"testing"

	"github.com/openai/openai-go/v3"
)

func TestStreamTokenCounter_ConsumeOpenAIChunk(t *testing.T) {
	counter, err := NewStreamTokenCounter()
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// Set initial input tokens
	counter.SetInputTokens(100)

	// Test content delta
	chunk := &openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Index: int64(0),
				Delta: openai.ChatCompletionChunkChoiceDelta{
					Content: "Hello, world!",
				},
			},
		},
	}

	inputTokens, outputTokens, err := counter.ConsumeOpenAIChunk(chunk)
	if err != nil {
		t.Fatalf("failed to consume chunk: %v", err)
	}

	if inputTokens != 100 {
		t.Errorf("expected input tokens 100, got %d", inputTokens)
	}

	// "Hello, world!" is approximately 3-4 tokens
	if outputTokens == 0 {
		t.Error("expected output tokens > 0")
	}

	t.Logf("Content 'Hello, world!' counted as %d tokens", outputTokens)
}

func TestStreamTokenCounter_ToolCall(t *testing.T) {
	counter, err := NewStreamTokenCounter()
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// Test tool call delta
	funcName := "get_weather"
	args := `{"city":"Tokyo"`

	chunk := &openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Index: int64(0),
				Delta: openai.ChatCompletionChunkChoiceDelta{
					ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{
						{
							Index: int64(0),
							Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{
								Name:      (funcName),
								Arguments: args,
							},
						},
					},
				},
			},
		},
	}

	_, outputTokens, err := counter.ConsumeOpenAIChunk(chunk)
	if err != nil {
		t.Fatalf("failed to consume chunk: %v", err)
	}

	if outputTokens == 0 {
		t.Error("expected output tokens > 0 for tool call")
	}

	t.Logf("Tool call counted as %d tokens", outputTokens)
}

func TestStreamTokenCounter_Usage(t *testing.T) {
	counter, err := NewStreamTokenCounter()
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// Test usage override - need to create valid JSON for Usage field
	chunkJSON := `{
		"id": "test",
		"object": "chat.completion.chunk",
		"created": 1234567890,
		"model": "gpt-4",
		"choices": [],
		"usage": {
			"prompt_tokens": 50,
			"completion_tokens": 25,
			"total_tokens": 75
		}
	}`

	var chunk openai.ChatCompletionChunk
	if err := chunk.UnmarshalJSON([]byte(chunkJSON)); err != nil {
		t.Fatalf("failed to unmarshal chunk: %v", err)
	}

	inputTokens, outputTokens, err := counter.ConsumeOpenAIChunk(&chunk)
	if err != nil {
		t.Fatalf("failed to consume chunk: %v", err)
	}

	if inputTokens != 50 {
		t.Errorf("expected input tokens 50, got %d", inputTokens)
	}
	if outputTokens != 25 {
		t.Errorf("expected output tokens 25, got %d", outputTokens)
	}
}

func TestStreamTokenCounter_ReasoningContent(t *testing.T) {
	counter, err := NewStreamTokenCounter()
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// Test reasoning content (o1-style)
	reasoning := "Let me think about this step by step."

	chunk := &openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Index: int64(0),
				Delta: openai.ChatCompletionChunkChoiceDelta{
					Refusal: (reasoning),
				},
			},
		},
	}

	_, outputTokens, err := counter.ConsumeOpenAIChunk(chunk)
	if err != nil {
		t.Fatalf("failed to consume chunk: %v", err)
	}

	if outputTokens == 0 {
		t.Error("expected output tokens > 0 for reasoning content")
	}

	t.Logf("Reasoning content counted as %d tokens", outputTokens)
}

func TestStreamTokenCounter_Reset(t *testing.T) {
	counter, err := NewStreamTokenCounter()
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	counter.SetInputTokens(100)
	counter.SetOutputTokens(50)

	if counter.TotalTokens() != 150 {
		t.Errorf("expected total 150, got %d", counter.TotalTokens())
	}

	counter.Reset()

	input, output := counter.GetCounts()
	if input != 0 || output != 0 {
		t.Errorf("expected reset to zero, got input=%d output=%d", input, output)
	}
}

func TestStreamTokenCounter_Concurrent(t *testing.T) {
	counter, err := NewStreamTokenCounter()
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// Test concurrent access
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			chunk := &openai.ChatCompletionChunk{
				Choices: []openai.ChatCompletionChunkChoice{
					{
						Index: int64(0),
						Delta: openai.ChatCompletionChunkChoiceDelta{
							Content: ("test content"),
						},
					},
				},
			}
			for j := 0; j < 100; j++ {
				counter.ConsumeOpenAIChunk(chunk)
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	_, output := counter.GetCounts()
	if output == 0 {
		t.Error("expected output tokens > 0 after concurrent updates")
	}
	t.Logf("Concurrent test: %d output tokens", output)
}

func TestStreamTokenCounter_HelperMethods(t *testing.T) {
	counter, err := NewStreamTokenCounter()
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// Test AddInputTokens
	counter.AddInputTokens(10)
	if counter.InputTokens() != 10 {
		t.Errorf("expected input tokens 10, got %d", counter.InputTokens())
	}

	// Test AddToOutputTokens
	counter.AddToOutputTokens(5)
	if counter.OutputTokens() != 5 {
		t.Errorf("expected output tokens 5, got %d", counter.OutputTokens())
	}

	// Test CountText
	text := "Hello, world!"
	counted := counter.CountText(text)
	if counted == 0 {
		t.Error("expected CountText to return > 0")
	}
	t.Logf("CountText(%q) = %d", text, counted)

	// Test TotalTokens
	if counter.TotalTokens() != 15 {
		t.Errorf("expected total 15, got %d", counter.TotalTokens())
	}
}

func TestStreamTokenCounter_ToolCallWithID(t *testing.T) {
	counter, err := NewStreamTokenCounter()
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// Test tool call with ID
	chunk := &openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Index: int64(0),
				Delta: openai.ChatCompletionChunkChoiceDelta{
					ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{
						{
							Index: int64(0),
							ID:    "call_abc123",
							Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{
								Name:      "my_function",
								Arguments: `{"param":"value"`,
							},
						},
					},
				},
			},
		},
	}

	_, outputTokens, err := counter.ConsumeOpenAIChunk(chunk)
	if err != nil {
		t.Fatalf("failed to consume chunk: %v", err)
	}

	if outputTokens == 0 {
		t.Error("expected output tokens > 0 for tool call with ID")
	}

	t.Logf("Tool call with ID counted as %d tokens", outputTokens)
}

func TestStreamTokenCounter_EmptyChunk(t *testing.T) {
	counter, err := NewStreamTokenCounter()
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}

	// Test empty chunk
	chunk := &openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Index: int64(0),
				Delta: openai.ChatCompletionChunkChoiceDelta{},
			},
		},
	}

	inputTokens, outputTokens, err := counter.ConsumeOpenAIChunk(chunk)
	if err != nil {
		t.Fatalf("failed to consume empty chunk: %v", err)
	}

	if inputTokens != 0 || outputTokens != 0 {
		t.Errorf("expected zero tokens for empty chunk, got input=%d output=%d", inputTokens, outputTokens)
	}
}
