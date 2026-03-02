package virtualmodel

import (
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
