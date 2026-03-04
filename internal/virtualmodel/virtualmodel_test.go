package virtualmodel

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewVirtualModel(t *testing.T) {
	cfg := &VirtualModelConfig{
		ID:          "test-model",
		Name:        "Test Model",
		Description: "A test model",
		Content:     "Test response",
		Delay:       100 * time.Millisecond,
	}

	vm := NewVirtualModel(cfg)

	if vm.GetID() != "test-model" {
		t.Errorf("Expected ID 'test-model', got '%s'", vm.GetID())
	}

	if vm.GetName() != "Test Model" {
		t.Errorf("Expected Name 'Test Model', got '%s'", vm.GetName())
	}

	if vm.GetContent() != "Test response" {
		t.Errorf("Expected Content 'Test response', got '%s'", vm.GetContent())
	}

	if vm.GetDelay() != 100*time.Millisecond {
		t.Errorf("Expected Delay 100ms, got %v", vm.GetDelay())
	}
}

func TestNewVirtualModelDefaults(t *testing.T) {
	cfg := &VirtualModelConfig{
		ID:      "test-model-defaults",
		Content: "Test",
	}

	vm := NewVirtualModel(cfg)

	if vm.config.Role != "assistant" {
		t.Errorf("Expected default role 'assistant', got '%s'", vm.config.Role)
	}

	if vm.config.FinishReason != "stop" {
		t.Errorf("Expected default finish_reason 'stop', got '%s'", vm.config.FinishReason)
	}
}

func TestRegistry(t *testing.T) {
	registry := NewRegistry()

	cfg := &VirtualModelConfig{
		ID:          "registry-test",
		Name:        "Registry Test",
		Description: "Test registry",
		Content:     "Registry test content",
	}

	vm := NewVirtualModel(cfg)

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
	registry := NewRegistry()
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
	chunks := splitIntoChunks(content)

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
	cfg := &VirtualModelConfig{
		ID:      "service-test",
		Content: "Service test",
	}
	err := service.RegisterModel(NewVirtualModel(cfg))
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
	messages := []Message{
		{Role: "user", Content: "Hello world"},
		{Role: "assistant", Content: "Hi there"},
	}

	count := estimateTokens(messages)
	if count <= 0 {
		t.Errorf("Expected positive token count, got %d", count)
	}

	// Test empty string
	emptyCount := estimateTokensString("")
	if emptyCount != 0 {
		t.Errorf("Expected 0 tokens for empty string, got %d", emptyCount)
	}

	// Test rough estimate (should be approximately length/4)
	shortContent := "Hello world"
	shortCount := estimateTokensString(shortContent)
	expectedShort := (len(shortContent) + 3) / 4
	if shortCount != expectedShort {
		t.Errorf("Expected %d tokens for '%s', got %d", expectedShort, shortContent, shortCount)
	}

	// Test empty messages slice
	emptyMessages := []Message{}
	emptyMsgCount := estimateTokens(emptyMessages)
	if emptyMsgCount != 0 {
		t.Errorf("Expected 0 tokens for empty messages, got %d", emptyMsgCount)
	}

	// Test messages with empty content
	messagesWithEmpty := []Message{
		{Role: "user", Content: ""},
		{Role: "assistant", Content: "Hello"},
	}
	countWithEmpty := estimateTokens(messagesWithEmpty)
	if countWithEmpty <= 0 {
		t.Errorf("Expected positive token count with empty content message, got %d", countWithEmpty)
	}
}

func TestRegistryList(t *testing.T) {
	registry := NewRegistry()

	cfg1 := &VirtualModelConfig{
		ID:      "list-test-1",
		Content: "Test 1",
	}
	cfg2 := &VirtualModelConfig{
		ID:      "list-test-2",
		Content: "Test 2",
	}

	registry.Register(NewVirtualModel(cfg1))
	registry.Register(NewVirtualModel(cfg2))

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
	registry := NewRegistry()

	cfg := &VirtualModelConfig{
		ID:      "clear-test",
		Content: "Test",
	}
	registry.Register(NewVirtualModel(cfg))

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

// Test unified type system
func TestVirtualModelTypes(t *testing.T) {
	tests := []struct {
		name     string
		vmType   VirtualModelType
		isStatic bool
		isTool   bool
		isProxy  bool
	}{
		{"static", VirtualModelTypeStatic, true, false, false},
		{"tool", VirtualModelTypeTool, false, true, false},
		{"proxy", VirtualModelTypeProxy, false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &VirtualModelConfig{
				ID:   "test-" + tt.name,
				Type: tt.vmType,
			}
			vm := NewVirtualModel(cfg)

			if vm.GetType() != tt.vmType {
				t.Errorf("Expected type %s, got %s", tt.vmType, vm.GetType())
			}
			if vm.IsStatic() != tt.isStatic {
				t.Errorf("IsStatic() = %v, want %v", vm.IsStatic(), tt.isStatic)
			}
			if vm.IsTool() != tt.isTool {
				t.Errorf("IsTool() = %v, want %v", vm.IsTool(), tt.isTool)
			}
			if vm.IsProxy() != tt.isProxy {
				t.Errorf("IsProxy() = %v, want %v", vm.IsProxy(), tt.isProxy)
			}
		})
	}
}

// Test default type is static
func TestVirtualModelDefaultType(t *testing.T) {
	cfg := &VirtualModelConfig{
		ID:      "test-default",
		Content: "Test",
	}
	vm := NewVirtualModel(cfg)

	if vm.GetType() != VirtualModelTypeStatic {
		t.Errorf("Expected default type %s, got %s", VirtualModelTypeStatic, vm.GetType())
	}
}

// Test tool model with generic arguments
func TestToolModel(t *testing.T) {
	// Test ask_user_question tool
	cfg := &VirtualModelConfig{
		ID:   "test-ask",
		Type: VirtualModelTypeTool,
		ToolCall: &ToolCallConfig{
			Name: "ask_user_question",
			Arguments: map[string]interface{}{
				"question": "Which option?",
				"options": []map[string]string{
					{"label": "A", "value": "a"},
				},
			},
		},
	}

	vm := NewVirtualModel(cfg)

	if !vm.IsTool() {
		t.Error("Expected IsTool() to return true")
	}

	toolCall := vm.GetToolCall()
	if toolCall == nil {
		t.Fatal("Expected tool call config")
	}

	if toolCall.Name != "ask_user_question" {
		t.Errorf("Expected tool name 'ask_user_question', got '%s'", toolCall.Name)
	}

	if toolCall.Arguments["question"] != "Which option?" {
		t.Errorf("Expected question 'Which option?', got '%v'", toolCall.Arguments["question"])
	}

	// Test web_search tool (different tool type)
	cfg2 := &VirtualModelConfig{
		ID:   "test-search",
		Type: VirtualModelTypeTool,
		ToolCall: &ToolCallConfig{
			Name: "web_search",
			Arguments: map[string]interface{}{
				"query": "latest AI news",
			},
		},
	}

	vm2 := NewVirtualModel(cfg2)
	if vm2.GetToolCall().Name != "web_search" {
		t.Errorf("Expected tool name 'web_search', got '%s'", vm2.GetToolCall().Name)
	}
}

// Test proxy model
func TestProxyModel(t *testing.T) {
	cfg := &VirtualModelConfig{
		ID:            "test-proxy",
		Type:          VirtualModelTypeProxy,
		DelegateModel: "claude-3-5-sonnet",
	}

	vm := NewVirtualModel(cfg)

	if !vm.IsProxy() {
		t.Error("Expected IsProxy() to return true")
	}

	if vm.GetDelegateModel() != "claude-3-5-sonnet" {
		t.Errorf("Expected delegate model 'claude-3-5-sonnet', got '%s'", vm.GetDelegateModel())
	}
}

// Test all default model types are registered
func TestDefaultModelTypes(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterDefaults()

	testCases := []struct {
		id       string
		expected VirtualModelType
	}{
		{"virtual-gpt-4", VirtualModelTypeStatic},
		{"virtual-claude-3", VirtualModelTypeStatic},
		{"echo-model", VirtualModelTypeStatic},
		{"compact-thinking", VirtualModelTypeProxy},
		{"compact-round-only", VirtualModelTypeProxy},
		{"compact-round-files", VirtualModelTypeProxy},
		{"ask-user-question", VirtualModelTypeTool},
		{"ask-confirmation", VirtualModelTypeTool},
		{"web-search-example", VirtualModelTypeTool},
	}

	for _, tc := range testCases {
		vm := registry.Get(tc.id)
		if vm == nil {
			t.Errorf("Model '%s' not found", tc.id)
			continue
		}
		if vm.GetType() != tc.expected {
			t.Errorf("Model '%s': expected type %s, got %s", tc.id, tc.expected, vm.GetType())
		}
	}
}

// Test GetStreamChunks with default behavior (splits content)
func TestGetStreamChunksDefault(t *testing.T) {
	cfg := &VirtualModelConfig{
		ID:      "test-stream-default",
		Content: "Hello world test",
	}
	vm := NewVirtualModel(cfg)

	chunks := vm.GetStreamChunks()
	if len(chunks) == 0 {
		t.Error("Expected at least one chunk from default streaming")
	}

	// Reconstruct content
	reconstructed := ""
	for _, chunk := range chunks {
		reconstructed += chunk
	}
	if reconstructed != "Hello world test" {
		t.Errorf("Reconstructed chunks don't match original. Expected 'Hello world test', got '%s'", reconstructed)
	}
}

// Test GetStreamChunks with custom chunks
func TestGetStreamChunksCustom(t *testing.T) {
	customChunks := []string{"Custom", " ", "Chunks"}
	cfg := &VirtualModelConfig{
		ID:           "test-stream-custom",
		Content:      "Original",
		StreamChunks: customChunks,
	}
	vm := NewVirtualModel(cfg)

	chunks := vm.GetStreamChunks()
	if len(chunks) != len(customChunks) {
		t.Errorf("Expected %d chunks, got %d", len(customChunks), len(chunks))
	}

	for i, chunk := range chunks {
		if chunk != customChunks[i] {
			t.Errorf("Chunk %d: expected '%s', got '%s'", i, customChunks[i], chunk)
		}
	}
}

// Test GetStreamChunks with empty content
func TestGetStreamChunksEmpty(t *testing.T) {
	cfg := &VirtualModelConfig{
		ID:      "test-stream-empty",
		Content: "",
	}
	vm := NewVirtualModel(cfg)

	chunks := vm.GetStreamChunks()
	if len(chunks) == 0 {
		t.Error("Expected at least one chunk even for empty content")
	}
}

// Test ToModel conversion
func TestToModel(t *testing.T) {
	cfg := &VirtualModelConfig{
		ID:          "test-to-model",
		Name:        "Test Model Name",
		Description: "Test Description",
		Content:     "Test content",
	}
	vm := NewVirtualModel(cfg)

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

	if model.Created == 0 {
		t.Error("Expected non-zero created timestamp")
	}

	// Test with ID only (Name defaults to ID)
	cfg2 := &VirtualModelConfig{
		ID:      "id-only",
		Content: "Content",
	}
	vm2 := NewVirtualModel(cfg2)
	model2 := vm2.ToModel()

	if model2.ID != "id-only" {
		t.Errorf("Expected ID 'id-only', got '%s'", model2.ID)
	}
}

// Test GetName with and without explicit name
func TestGetName(t *testing.T) {
	// With explicit name
	cfg1 := &VirtualModelConfig{
		ID:   "test-id",
		Name: "Explicit Name",
	}
	vm1 := NewVirtualModel(cfg1)
	if vm1.GetName() != "Explicit Name" {
		t.Errorf("Expected 'Explicit Name', got '%s'", vm1.GetName())
	}

	// Without explicit name (should default to ID)
	cfg2 := &VirtualModelConfig{
		ID:   "test-id-only",
		Name: "",
	}
	vm2 := NewVirtualModel(cfg2)
	if vm2.GetName() != "test-id-only" {
		t.Errorf("Expected name to default to ID 'test-id-only', got '%s'", vm2.GetName())
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

	if req.Temperature == nil || *req.Temperature != 0.7 {
		t.Error("Expected temperature 0.7")
	}

	if req.MaxTokens == nil || *req.MaxTokens != 100 {
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

// Test Anthropic API types JSON serialization
func TestAnthropicMessageRequestJSON(t *testing.T) {
	jsonData := `{
		"model": "virtual-claude-3",
		"messages": [
			{"role": "user", "content": "Hello"}
		],
		"max_tokens": 1024,
		"stream": true
	}`

	var req AnthropicMessageRequest
	err := json.Unmarshal([]byte(jsonData), &req)
	if err != nil {
		t.Fatalf("Failed to unmarshal AnthropicMessageRequest: %v", err)
	}

	if req.Model != "virtual-claude-3" {
		t.Errorf("Expected model 'virtual-claude-3', got '%s'", req.Model)
	}

	if len(req.Messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(req.Messages))
	}

	if !req.Stream {
		t.Error("Expected stream to be true")
	}
}

// Test AnthropicMessageResponse JSON serialization
func TestAnthropicMessageResponseJSON(t *testing.T) {
	resp := AnthropicMessageResponse{
		ID:         "msg_test_123",
		Type:       "message",
		Role:       "assistant",
		Model:      "virtual-claude-3",
		StopReason: "end_turn",
		Content: []AnthropicContent{
			{Type: "text", Text: "Test response"},
		},
		Usage: AnthropicUsage{
			InputTokens:  15,
			OutputTokens: 25,
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal AnthropicMessageResponse: %v", err)
	}

	var unmarshaled AnthropicMessageResponse
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal AnthropicMessageResponse: %v", err)
	}

	if unmarshaled.ID != resp.ID {
		t.Errorf("ID mismatch: expected '%s', got '%s'", resp.ID, unmarshaled.ID)
	}

	if len(unmarshaled.Content) != 1 {
		t.Fatalf("Expected 1 content block, got %d", len(unmarshaled.Content))
	}

	if unmarshaled.Content[0].Type != "text" {
		t.Errorf("Expected content type 'text', got '%s'", unmarshaled.Content[0].Type)
	}
}

// Test ToolCallConfig JSON serialization
func TestToolCallConfigJSON(t *testing.T) {
	cfg := ToolCallConfig{
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

	var unmarshaled ToolCallConfig
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
