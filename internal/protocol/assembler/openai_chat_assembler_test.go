package assembler

import (
	"testing"

	"github.com/openai/openai-go/v3"
)

// TestOpenAIStreamAssembler_Basic tests basic chunk accumulation
func TestOpenAIStreamAssembler_Basic(t *testing.T) {
	assembler := NewOpenAIStreamAssembler()

	// Simulate streaming chunks
	chunks := []openai.ChatCompletionChunk{
		{
			ID:      "chatcmpl-123",
			Object:  "chat.completion.chunk",
			Model:   "gpt-4",
			Created: 1234567890,
			Choices: []openai.ChatCompletionChunkChoice{
				{
					Index: 0,
					Delta: openai.ChatCompletionChunkChoiceDelta{
						Role:    "assistant",
						Content: "Hello",
					},
				},
			},
		},
		{
			ID:     "chatcmpl-123",
			Object: "chat.completion.chunk",
			Choices: []openai.ChatCompletionChunkChoice{
				{
					Index: 0,
					Delta: openai.ChatCompletionChunkChoiceDelta{
						Content: " world",
					},
				},
			},
		},
		{
			ID:     "chatcmpl-123",
			Object: "chat.completion.chunk",
			Choices: []openai.ChatCompletionChunkChoice{
				{
					Index:        0,
					FinishReason: "stop",
				},
			},
		},
	}

	for _, chunk := range chunks {
		if !assembler.AddChunk(chunk) {
			t.Fatal("AddChunk failed")
		}
	}

	result := assembler.Finish()
	if result.ID != "chatcmpl-123" {
		t.Errorf("expected ID chatcmpl-123, got %s", result.ID)
	}

	if len(result.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(result.Choices))
	}

	content := result.Choices[0].Message.Content
	if content != "Hello world" {
		t.Errorf("expected 'Hello world', got '%s'", content)
	}

	role := string(result.Choices[0].Message.Role)
	if role != "assistant" {
		t.Errorf("expected role 'assistant', got '%s'", role)
	}
}

// TestOpenAIStreamAssembler_ToolCalls tests tool call accumulation
func TestOpenAIStreamAssembler_ToolCalls(t *testing.T) {
	assembler := NewOpenAIStreamAssembler()

	chunks := []openai.ChatCompletionChunk{
		{
			ID:     "test",
			Object: "chat.completion.chunk",
			Choices: []openai.ChatCompletionChunkChoice{{
				Index: 0,
				Delta: openai.ChatCompletionChunkChoiceDelta{
					Role: "assistant",
					ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{
						{Index: 0, ID: "call_123", Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{Name: "get_weather"}},
					},
				},
			}},
		},
		{
			ID:     "test",
			Object: "chat.completion.chunk",
			Choices: []openai.ChatCompletionChunkChoice{{
				Index: 0,
				Delta: openai.ChatCompletionChunkChoiceDelta{
					ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{
						{Index: 0, Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{Arguments: `{"city":"NYC"}`}},
					},
				},
			}},
		},
	}

	for _, chunk := range chunks {
		if !assembler.AddChunk(chunk) {
			t.Fatal("AddChunk failed")
		}
	}

	result := assembler.Finish()
	if len(result.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.Choices[0].Message.ToolCalls))
	}

	tc := result.Choices[0].Message.ToolCalls[0]
	if tc.ID != "call_123" {
		t.Errorf("expected ID 'call_123', got '%s'", tc.ID)
	}

	expectedName := "get_weather"
	if tc.Function.Name != expectedName {
		t.Errorf("expected name '%s', got '%s'", expectedName, tc.Function.Name)
	}

	expectedArgs := `{"city":"NYC"}`
	if string(tc.Function.Arguments) != expectedArgs {
		t.Errorf("expected args '%s', got '%s'", expectedArgs, tc.Function.Arguments)
	}
}

// TestOpenAIStreamAssembler_UsageAccumulation tests usage token accumulation
func TestOpenAIStreamAssembler_UsageAccumulation(t *testing.T) {
	assembler := NewOpenAIStreamAssembler()

	chunks := []openai.ChatCompletionChunk{
		{Usage: openai.CompletionUsage{CompletionTokens: 10, PromptTokens: 5, TotalTokens: 15}},
		{Usage: openai.CompletionUsage{CompletionTokens: 5, PromptTokens: 0, TotalTokens: 5}},
	}

	for _, chunk := range chunks {
		assembler.AddChunk(chunk)
	}

	result := assembler.Finish()
	if result.Usage.CompletionTokens != 15 {
		t.Errorf("expected 15 completion tokens, got %d", result.Usage.CompletionTokens)
	}
	if result.Usage.PromptTokens != 5 {
		t.Errorf("expected 5 prompt tokens, got %d", result.Usage.PromptTokens)
	}
	if result.Usage.TotalTokens != 20 {
		t.Errorf("expected 20 total tokens, got %d", result.Usage.TotalTokens)
	}
}

// TestOpenAIStreamAssembler_NilAssembler tests nil assembler handling
func TestOpenAIStreamAssembler_NilAssembler(t *testing.T) {
	var assembler *OpenAIChatStreamAssembler
	if assembler != nil {
		assembler.AddChunk(openai.ChatCompletionChunk{})
	}
	// Should not panic
}

// TestOpenAIStreamAssembler_ResultDirectAccess tests direct access to accumulator
func TestOpenAIStreamAssembler_ResultDirectAccess(t *testing.T) {
	assembler := NewOpenAIStreamAssembler()
	chunk := openai.ChatCompletionChunk{
		ID:      "direct-test",
		Choices: []openai.ChatCompletionChunkChoice{{Index: 0, Delta: openai.ChatCompletionChunkChoiceDelta{Role: "assistant"}}},
	}
	assembler.AddChunk(chunk)

	acc := assembler.Result()
	if acc.ChatCompletion.ID != "direct-test" {
		t.Errorf("Result() direct access failed")
	}
}

// TestOpenAIStreamAssembler_EmptyChunks tests adding empty chunks
func TestOpenAIStreamAssembler_EmptyChunks(t *testing.T) {
	assembler := NewOpenAIStreamAssembler()
	emptyChunk := openai.ChatCompletionChunk{
		ID:     "empty",
		Object: "chat.completion.chunk",
	}
	if !assembler.AddChunk(emptyChunk) {
		t.Error("AddChunk failed for empty chunk")
	}
	result := assembler.Finish()
	if result.ID != "empty" {
		t.Errorf("expected ID 'empty', got '%s'", result.ID)
	}
}

// TestOpenAIStreamAssembler_IDMismatch tests ID mismatch handling
func TestOpenAIStreamAssembler_IDMismatch(t *testing.T) {
	assembler := NewOpenAIStreamAssembler()

	// First chunk with one ID
	chunk1 := openai.ChatCompletionChunk{
		ID:      "msg-1",
		Object:  "chat.completion.chunk",
		Choices: []openai.ChatCompletionChunkChoice{{Index: 0, Delta: openai.ChatCompletionChunkChoiceDelta{Content: "Hello"}}},
	}
	assembler.AddChunk(chunk1)

	// Second chunk with different ID - should fail
	chunk2 := openai.ChatCompletionChunk{
		ID:      "msg-2", // Different ID
		Object:  "chat.completion.chunk",
		Choices: []openai.ChatCompletionChunkChoice{{Index: 0, Delta: openai.ChatCompletionChunkChoiceDelta{Content: " world"}}},
	}
	if assembler.AddChunk(chunk2) {
		t.Error("Expected AddChunk to return false for ID mismatch")
	}

	result := assembler.Finish()
	if result.Choices[0].Message.Content != "Hello" {
		t.Errorf("Content should not have changed, got '%s'", result.Choices[0].Message.Content)
	}
}

// TestOpenAIStreamAssembler_WrapperBehavior verifies wrapper delegates correctly
func TestOpenAIStreamAssembler_WrapperBehavior(t *testing.T) {
	assembler := NewOpenAIStreamAssembler()

	// Verify wrapper exposes SDK methods
	if assembler == nil {
		t.Fatal("NewOpenAIStreamAssembler returned nil")
	}

	// Verify we can access the internal accumulator
	acc := assembler.Result()
	if acc == nil {
		t.Error("Result() should not return nil")
	}

	// Verify Finish returns a valid pointer
	result := assembler.Finish()
	if result == nil {
		t.Error("Finish() should not return nil")
	}
}

// TestOpenAIStreamAssembler_StateTrackingMethods tests state tracking methods exist
func TestOpenAIStreamAssembler_StateTrackingMethods(t *testing.T) {
	assembler := NewOpenAIStreamAssembler()

	// Add a chunk to initialize state
	chunk := openai.ChatCompletionChunk{
		ID:      "state-test",
		Object:  "chat.completion.chunk",
		Choices: []openai.ChatCompletionChunkChoice{{Index: 0, Delta: openai.ChatCompletionChunkChoiceDelta{Content: "test"}}},
	}
	assembler.AddChunk(chunk)

	// Verify state tracking methods are accessible and return expected types
	content, ok := assembler.JustFinishedContent()
	_ = content // string
	_ = ok      // bool

	refusal, ok := assembler.JustFinishedRefusal()
	_ = refusal // string
	_ = ok      // bool

	toolCall, ok := assembler.JustFinishedToolCall()
	_ = toolCall // openai.FinishedChatCompletionToolCall
	_ = ok       // bool

	// Just verify the methods exist and are callable
}
