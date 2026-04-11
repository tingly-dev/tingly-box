package virtualserver

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/internal/virtualmodel"
)

func TestNewMockModel(t *testing.T) {
	vm := virtualmodel.NewMockModel(&virtualmodel.MockModelConfig{
		ID:      "test-model",
		Name:    "Test Model",
		Content: "Test response",
		Delay:   100 * time.Millisecond,
	})

	if vm.GetID() != "test-model" {
		t.Errorf("Expected ID 'test-model', got '%s'", vm.GetID())
	}

	if vm.SimulatedDelay() != 100*time.Millisecond {
		t.Errorf("Expected Delay 100ms, got %v", vm.SimulatedDelay())
	}
}

func TestNewMockModelDefaults(t *testing.T) {
	vm := virtualmodel.NewMockModel(&virtualmodel.MockModelConfig{
		ID:      "test-model-defaults",
		Content: "Test",
	})

	resp, _ := vm.HandleAnthropic(nil)
	if resp.StopReason == "" {
		t.Error("Expected non-empty default StopReason")
	}
}

func TestRegistry(t *testing.T) {
	registry := virtualmodel.NewRegistry()

	vm := virtualmodel.NewMockModel(&virtualmodel.MockModelConfig{
		ID:      "registry-test",
		Content: "Registry test content",
	})

	// Test Register
	err := registry.Register(vm)
	if err != nil {
		t.Fatalf("Failed to register model: %v", err)
	}

	// Test duplicate registration
	err = registry.Register(vm)
	if err == nil {
		t.Error("Expected error when registering duplicate model, got nil")
	}

	// Test Get
	retrieved := registry.Get("registry-test")
	if retrieved == nil {
		t.Error("Failed to retrieve registered model")
	}

	if retrieved.GetID() != "registry-test" {
		t.Errorf("Retrieved wrong model, expected ID 'registry-test', got '%s'", retrieved.GetID())
	}

	// Test ListModels
	models := registry.ListModels()
	if len(models) != 1 {
		t.Errorf("Expected 1 model, got %d", len(models))
	}

	if models[0].ID != "registry-test" {
		t.Errorf("Expected model ID 'registry-test', got '%s'", models[0].ID)
	}

	// Test Unregister
	registry.Unregister("registry-test")
	retrieved = registry.Get("registry-test")
	if retrieved != nil {
		t.Error("Model should be unregistered")
	}
}

func TestRegisterDefaults(t *testing.T) {
	registry := virtualmodel.NewRegistry()
	registry.RegisterDefaults()

	models := registry.ListModels()
	if len(models) < 3 {
		t.Errorf("Expected at least 3 default models, got %d", len(models))
	}

	// Check for expected default models
	expectedModels := []string{"virtual-gpt-4", "virtual-claude-3", "echo-model"}
	modelIDs := make(map[string]bool)
	for _, m := range models {
		modelIDs[m.ID] = true
	}

	for _, expected := range expectedModels {
		if !modelIDs[expected] {
			t.Errorf("Expected default model '%s' not found", expected)
		}
	}
}

func TestSplitIntoChunks(t *testing.T) {
	content := "Hello world this is a test"
	chunks := token.SplitIntoChunks(content)

	if len(chunks) == 0 {
		t.Error("Expected at least one chunk, got none")
	}

	// Reconstruct and verify
	reconstructed := ""
	for _, chunk := range chunks {
		reconstructed += chunk
	}

	if reconstructed != content {
		t.Errorf("Reconstructed content doesn't match original.\nExpected: %s\nGot: %s", content, reconstructed)
	}
}

func TestService(t *testing.T) {
	service := NewService()

	// Test default models are registered
	models := service.ListModels()
	if len(models) < 3 {
		t.Errorf("Expected at least 3 default models, got %d", len(models))
	}

	// Test GetModel
	vm := service.GetModel("virtual-gpt-4")
	if vm == nil {
		t.Error("Failed to get default virtual-gpt-4 model")
	}

	// Test RegisterModel
	err := service.RegisterModel(virtualmodel.NewMockModel(&virtualmodel.MockModelConfig{
		ID:      "service-test",
		Content: "Service test",
	}))
	if err != nil {
		t.Fatalf("Failed to register model: %v", err)
	}

	// Verify model was registered
	vm = service.GetModel("service-test")
	if vm == nil {
		t.Error("Failed to retrieve newly registered model")
	}

	// Test UnregisterModel
	service.UnregisterModel("service-test")
	vm = service.GetModel("service-test")
	if vm != nil {
		t.Error("Model should be unregistered")
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

	// Test empty string
	emptyCount := token.EstimateTokensString("")
	if emptyCount != 0 {
		t.Errorf("Expected 0 tokens for empty string, got %d", emptyCount)
	}

	// Test rough estimate (should be approximately length/4)
	shortContent := "Hello world"
	shortCount := token.EstimateTokensString(shortContent)
	expectedShort := int64((len(shortContent) + 3) / 4)
	if shortCount != expectedShort {
		t.Errorf("Expected %d tokens for '%s', got %d", expectedShort, shortContent, shortCount)
	}

	// Test empty messages slice
	emptyMessages := []openai.ChatCompletionMessageParamUnion{}
	emptyMsgCount := token.EstimateMessagesTokens(emptyMessages)
	if emptyMsgCount != 0 {
		t.Errorf("Expected 0 tokens for empty messages, got %d", emptyMsgCount)
	}

	// Test messages with empty content
	messagesWithEmpty := []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage(""),
		openai.AssistantMessage("Hello"),
	}
	countWithEmpty := token.EstimateMessagesTokens(messagesWithEmpty)
	if countWithEmpty <= 0 {
		t.Errorf("Expected positive token count with empty content message, got %d", countWithEmpty)
	}
}

func TestRegistryList(t *testing.T) {
	registry := virtualmodel.NewRegistry()

	registry.Register(virtualmodel.NewMockModel(&virtualmodel.MockModelConfig{ID: "list-test-1", Content: "Test 1"}))
	registry.Register(virtualmodel.NewMockModel(&virtualmodel.MockModelConfig{ID: "list-test-2", Content: "Test 2"}))

	// Test List() returns []*VirtualModel
	vms := registry.List()
	if len(vms) != 2 {
		t.Errorf("Expected 2 virtual models from List(), got %d", len(vms))
	}

	// Verify returned items have correct methods
	if vms[0].GetID() != "list-test-1" {
		t.Errorf("Expected first model ID 'list-test-1', got '%s'", vms[0].GetID())
	}
	if vms[1].GetID() != "list-test-2" {
		t.Errorf("Expected second model ID 'list-test-2', got '%s'", vms[1].GetID())
	}
}

func TestRegistryClear(t *testing.T) {
	registry := virtualmodel.NewRegistry()

	registry.Register(virtualmodel.NewMockModel(&virtualmodel.MockModelConfig{ID: "clear-test", Content: "Test"}))

	// Verify model is registered
	if registry.Get("clear-test") == nil {
		t.Fatal("Model should be registered before clear")
	}

	// Clear all models
	registry.Clear()

	// Verify all models are gone
	if registry.Get("clear-test") != nil {
		t.Error("Model should be removed after clear")
	}

	models := registry.ListModels()
	if len(models) != 0 {
		t.Errorf("Expected 0 models after clear, got %d", len(models))
	}

	vms := registry.List()
	if len(vms) != 0 {
		t.Errorf("Expected 0 virtual models from List() after clear, got %d", len(vms))
	}
}

// Test that MockModel and TransformModel both satisfy VirtualModel interface
func TestVirtualModelTypes(t *testing.T) {
	t.Run("MockModel satisfies VirtualModel", func(t *testing.T) {
		var vm virtualmodel.VirtualModel = virtualmodel.NewMockModel(&virtualmodel.MockModelConfig{
			ID:      "test-mock",
			Content: "hello",
		})
		if vm.GetID() != "test-mock" {
			t.Errorf("Expected ID 'test-mock', got '%s'", vm.GetID())
		}
	})

	t.Run("MockModel satisfies AnthropicVirtualModel", func(t *testing.T) {
		vm := virtualmodel.NewMockModel(&virtualmodel.MockModelConfig{
			ID:      "test-mock-anthropic",
			Content: "hello",
		})
		var _ virtualmodel.AnthropicVirtualModel = vm
	})

	t.Run("MockModel satisfies OpenAIChatVirtualModel", func(t *testing.T) {
		vm := virtualmodel.NewMockModel(&virtualmodel.MockModelConfig{
			ID:      "test-mock-openai",
			Content: "hello",
		})
		var _ virtualmodel.OpenAIChatVirtualModel = vm
	})

	t.Run("MockModel tool satisfies VirtualModel", func(t *testing.T) {
		var vm virtualmodel.VirtualModel = virtualmodel.NewMockModel(&virtualmodel.MockModelConfig{
			ID: "test-tool",
			ToolCall: &virtualmodel.ToolCallConfig{
				Name:      "web_search",
				Arguments: map[string]interface{}{"query": "test"},
			},
		})
		if vm.GetID() != "test-tool" {
			t.Errorf("Expected ID 'test-tool', got '%s'", vm.GetID())
		}
	})

	t.Run("TransformModel satisfies VirtualModel", func(t *testing.T) {
		var vm virtualmodel.VirtualModel = virtualmodel.NewTransformModel(&virtualmodel.TransformModelConfig{
			ID: "test-transform",
		})
		if vm.GetID() != "test-transform" {
			t.Errorf("Expected ID 'test-transform', got '%s'", vm.GetID())
		}
	})

	t.Run("TransformModel satisfies AnthropicVirtualModel", func(t *testing.T) {
		vm := virtualmodel.NewTransformModel(&virtualmodel.TransformModelConfig{ID: "test-transform-a"})
		var _ virtualmodel.AnthropicVirtualModel = vm
	})

	t.Run("TransformModel does NOT satisfy OpenAIChatVirtualModel", func(t *testing.T) {
		var vm virtualmodel.VirtualModel = virtualmodel.NewTransformModel(&virtualmodel.TransformModelConfig{ID: "t"})
		if _, ok := vm.(virtualmodel.OpenAIChatVirtualModel); ok {
			t.Error("TransformModel must not implement OpenAIChatVirtualModel")
		}
	})
}

// Test MockModel default stop reason
func TestVirtualModelDefaultType(t *testing.T) {
	vm := virtualmodel.NewMockModel(&virtualmodel.MockModelConfig{
		ID:      "test-default",
		Content: "Test",
	})
	resp, _ := vm.HandleAnthropic(nil)
	if resp.StopReason == "" {
		t.Error("Expected non-empty default StopReason")
	}
	if string(resp.StopReason) != "stop" {
		t.Errorf("Expected default StopReason 'stop', got '%s'", resp.StopReason)
	}
}

// Test MockModel tool response contains correct tool call
func TestToolModel(t *testing.T) {
	vm := virtualmodel.NewMockModel(&virtualmodel.MockModelConfig{
		ID: "test-ask",
		ToolCall: &virtualmodel.ToolCallConfig{
			Name: "ask_user_question",
			Arguments: map[string]interface{}{
				"question": "Which option?",
				"options": []map[string]string{
					{"label": "A", "value": "a"},
				},
			},
		},
	})

	resp, _ := vm.HandleAnthropic(nil)
	if string(resp.StopReason) != "tool_use" {
		t.Errorf("Expected StopReason 'tool_use', got '%s'", resp.StopReason)
	}

	// Find the tool_use block
	var foundToolUse bool
	for _, blk := range resp.Content {
		if blk.OfToolUse != nil && blk.OfToolUse.Name == "ask_user_question" {
			foundToolUse = true
		}
	}
	if !foundToolUse {
		t.Error("Expected tool_use block with name 'ask_user_question'")
	}

	// Test another tool type
	vm2 := virtualmodel.NewMockModel(&virtualmodel.MockModelConfig{
		ID: "test-search",
		ToolCall: &virtualmodel.ToolCallConfig{
			Name:      "web_search",
			Arguments: map[string]interface{}{"query": "latest AI news"},
		},
	})
	resp2, _ := vm2.HandleAnthropic(nil)
	var foundSearch bool
	for _, blk := range resp2.Content {
		if blk.OfToolUse != nil && blk.OfToolUse.Name == "web_search" {
			foundSearch = true
		}
	}
	if !foundSearch {
		t.Error("Expected tool_use block with name 'web_search'")
	}
}

// Test TransformModel SimulatedDelay is 0
func TestProxyModel(t *testing.T) {
	vm := virtualmodel.NewTransformModel(&virtualmodel.TransformModelConfig{
		ID: "test-proxy",
	})

	if vm.SimulatedDelay() != 0 {
		t.Errorf("Expected SimulatedDelay 0 for TransformModel, got %v", vm.SimulatedDelay())
	}
}

// Test all default models are registered
func TestDefaultModelTypes(t *testing.T) {
	registry := virtualmodel.NewRegistry()
	registry.RegisterDefaults()

	expectedIDs := []string{
		"virtual-gpt-4",
		"virtual-claude-3",
		"echo-model",
		"compact-thinking",
		"compact-round-only",
		"compact-round-files",
		"ask-user-question",
		"ask-confirmation",
		"web-search-example",
	}

	for _, id := range expectedIDs {
		vm := registry.Get(id)
		if vm == nil {
			t.Errorf("Model '%s' not found", id)
		}
	}
}

// Test ToModel conversion
func TestToModel(t *testing.T) {
	vm := virtualmodel.NewMockModel(&virtualmodel.MockModelConfig{
		ID:      "test-to-model",
		Content: "Test content",
	})

	model := vm.ToModel()

	if model.ID != "test-to-model" {
		t.Errorf("Expected model ID 'test-to-model', got '%s'", model.ID)
	}

	if model.Object != "model" {
		t.Errorf("Expected object 'model', got '%s'", model.Object)
	}

	if model.OwnedBy != "tingly-box-virtual" {
		t.Errorf("Expected owned_by 'tingly-box-virtual', got '%s'", model.OwnedBy)
	}

	// Test with another ID
	vm2 := virtualmodel.NewMockModel(&virtualmodel.MockModelConfig{
		ID:      "id-only",
		Content: "Content",
	})
	model2 := vm2.ToModel()

	if model2.ID != "id-only" {
		t.Errorf("Expected ID 'id-only', got '%s'", model2.ID)
	}
}

// Test GetID for MockModel
func TestGetName(t *testing.T) {
	vm := virtualmodel.NewMockModel(&virtualmodel.MockModelConfig{
		ID:      "test-id",
		Content: "Test",
	})
	if vm.GetID() != "test-id" {
		t.Errorf("Expected ID 'test-id', got '%s'", vm.GetID())
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

	// Test marshaling
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
	cfg := virtualmodel.ToolCallConfig{
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

	var unmarshaled virtualmodel.ToolCallConfig
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
