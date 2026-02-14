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
