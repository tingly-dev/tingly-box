package virtualserver

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"

	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/vmodel"
	openaivm "github.com/tingly-dev/tingly-box/vmodel/openai"
)

// TestService_DefaultRegistries verifies that the Service exposes both
// per-protocol registries with their default models pre-loaded.
func TestService_DefaultRegistries(t *testing.T) {
	service := NewService()

	if got := service.GetAnthropicRegistry().Get("virtual-claude-3"); got == nil {
		t.Error("Anthropic registry should contain 'virtual-claude-3'")
	}
	if got := service.GetOpenAIRegistry().Get("virtual-gpt-4"); got == nil {
		t.Error("OpenAI registry should contain 'virtual-gpt-4'")
	}

	// virtual-gpt-4 must NOT be in the Anthropic registry.
	if got := service.GetAnthropicRegistry().Get("virtual-gpt-4"); got != nil {
		t.Error("Anthropic registry must not contain 'virtual-gpt-4'")
	}
	// virtual-claude-3 must NOT be in the OpenAI registry.
	if got := service.GetOpenAIRegistry().Get("virtual-claude-3"); got != nil {
		t.Error("OpenAI registry must not contain 'virtual-claude-3'")
	}
}

// TestService_RegisterModel checks that custom models can be added to the
// OpenAI registry through the exposed registry.
func TestService_RegisterModel(t *testing.T) {
	service := NewService()
	reg := service.GetOpenAIRegistry()

	err := reg.Register(openaivm.NewMockModel(&openaivm.MockModelConfig{
		ID:      "service-test",
		Content: "Service test",
		Delay:   10 * time.Millisecond,
	}))
	if err != nil {
		t.Fatalf("Failed to register model: %v", err)
	}

	if vm := reg.Get("service-test"); vm == nil {
		t.Error("Failed to retrieve newly registered model")
	}

	reg.Unregister("service-test")
	if vm := reg.Get("service-test"); vm != nil {
		t.Error("Model should be unregistered")
	}
}

// TestSplitIntoChunks verifies the token chunk helper round-trips content.
func TestSplitIntoChunks(t *testing.T) {
	content := "Hello world this is a test"
	chunks := token.SplitIntoChunks(content)

	if len(chunks) == 0 {
		t.Error("Expected at least one chunk, got none")
	}

	reconstructed := ""
	for _, chunk := range chunks {
		reconstructed += chunk
	}

	if reconstructed != content {
		t.Errorf("Reconstructed content doesn't match original.\nExpected: %s\nGot: %s", content, reconstructed)
	}
}

func TestEstimateTokens(t *testing.T) {
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage("Hello world"),
		openai.AssistantMessage("Hi there"),
	}

	count := token.EstimateMessagesTokens(messages)
	if count <= 0 {
		t.Errorf("Expected positive token count, got %d", count)
	}

	emptyCount := token.EstimateTokensString("")
	if emptyCount != 0 {
		t.Errorf("Expected 0 tokens for empty string, got %d", emptyCount)
	}

	shortContent := "Hello world"
	shortCount := token.EstimateTokensString(shortContent)
	expectedShort := int64((len(shortContent) + 3) / 4)
	if shortCount != expectedShort {
		t.Errorf("Expected %d tokens for '%s', got %d", expectedShort, shortContent, shortCount)
	}

	emptyMessages := []openai.ChatCompletionMessageParamUnion{}
	emptyMsgCount := token.EstimateMessagesTokens(emptyMessages)
	if emptyMsgCount != 0 {
		t.Errorf("Expected 0 tokens for empty messages, got %d", emptyMsgCount)
	}

	messagesWithEmpty := []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage(""),
		openai.AssistantMessage("Hello"),
	}
	countWithEmpty := token.EstimateMessagesTokens(messagesWithEmpty)
	if countWithEmpty <= 0 {
		t.Errorf("Expected positive token count with empty content message, got %d", countWithEmpty)
	}
}

// Test API types JSON serialization
func TestChatCompletionRequestJSON(t *testing.T) {
	jsonData := `{
		"messages": [
			{"role": "user", "content": "Hello"},
			{"role": "assistant", "content": "Hi there"}
		],
		"model": "virtual-gpt-4",
		"temperature": 0.7,
		"max_tokens": 100,
		"stream": false
	}`

	var req ChatCompletionRequest
	err := json.Unmarshal([]byte(jsonData), &req)
	if err != nil {
		t.Fatalf("Failed to unmarshal ChatCompletionRequest: %v", err)
	}

	if len(req.Messages) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(req.Messages))
	}

	if req.Model != "virtual-gpt-4" {
		t.Errorf("Expected model 'virtual-gpt-4', got '%s'", req.Model)
	}

	if param.IsOmitted(req.Temperature) || req.Temperature.Value != 0.7 {
		t.Error("Expected temperature 0.7")
	}

	if param.IsOmitted(req.MaxTokens) || req.MaxTokens.Value != 100 {
		t.Error("Expected max_tokens 100")
	}

	if req.Stream {
		t.Error("Expected stream to be false")
	}

	output, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal ChatCompletionRequest: %v", err)
	}

	var req2 ChatCompletionRequest
	err = json.Unmarshal(output, &req2)
	if err != nil {
		t.Fatalf("Failed to unmarshal marshaled data: %v", err)
	}

	if req2.Model != req.Model {
		t.Error("Model mismatch after round-trip")
	}
}

// Test Message with tool calls JSON serialization
func TestMessageWithToolCallsJSON(t *testing.T) {
	jsonData := `{
		"role": "assistant",
		"content": "Let me search for that",
		"tool_calls": [
			{
				"id": "call_123",
				"type": "function",
				"function": {
					"name": "web_search",
					"arguments": "{\"query\":\"test\"}"
				}
			}
		]
	}`

	var msg Message
	err := json.Unmarshal([]byte(jsonData), &msg)
	if err != nil {
		t.Fatalf("Failed to unmarshal Message with tool_calls: %v", err)
	}

	if msg.Role != "assistant" {
		t.Errorf("Expected role 'assistant', got '%s'", msg.Role)
	}

	if len(msg.ToolCalls) != 1 {
		t.Fatalf("Expected 1 tool call, got %d", len(msg.ToolCalls))
	}

	if msg.ToolCalls[0].ID != "call_123" {
		t.Errorf("Expected tool call ID 'call_123', got '%s'", msg.ToolCalls[0].ID)
	}

	if msg.ToolCalls[0].Function.Name != "web_search" {
		t.Errorf("Expected function name 'web_search', got '%s'", msg.ToolCalls[0].Function.Name)
	}
}

// Test ChatCompletionResponse JSON serialization
func TestChatCompletionResponseJSON(t *testing.T) {
	resp := ChatCompletionResponse{
		ID:      "chatcmpl-test",
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   "virtual-gpt-4",
		Choices: []Choice{
			{
				Index: 0,
				Message: Message{
					Role:    "assistant",
					Content: "Test response",
				},
				FinishReason: "stop",
			},
		},
		Usage: Usage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal ChatCompletionResponse: %v", err)
	}

	var unmarshaled ChatCompletionResponse
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal ChatCompletionResponse: %v", err)
	}

	if unmarshaled.ID != resp.ID {
		t.Errorf("ID mismatch: expected '%s', got '%s'", resp.ID, unmarshaled.ID)
	}

	if unmarshaled.Usage.TotalTokens != 30 {
		t.Errorf("TotalTokens mismatch: expected 30, got %d", unmarshaled.Usage.TotalTokens)
	}
}

// Test ToolCallConfig JSON serialization
func TestToolCallConfigJSON(t *testing.T) {
	cfg := vmodel.ToolCallConfig{
		Name: "ask_user_question",
		Arguments: map[string]interface{}{
			"question": "Choose an option",
			"options": []map[string]string{
				{"label": "Option A", "value": "a"},
				{"label": "Option B", "value": "b"},
			},
		},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Failed to marshal ToolCallConfig: %v", err)
	}

	var unmarshaled vmodel.ToolCallConfig
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal ToolCallConfig: %v", err)
	}

	if unmarshaled.Name != cfg.Name {
		t.Errorf("Name mismatch: expected '%s', got '%s'", cfg.Name, unmarshaled.Name)
	}

	if unmarshaled.Arguments["question"] != "Choose an option" {
		t.Error("Arguments not preserved correctly")
	}
}
