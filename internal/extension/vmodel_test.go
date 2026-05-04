package extension

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/virtualmodel"
	anthropicvm "github.com/tingly-dev/tingly-box/internal/virtualmodel/anthropic"
	openaivm "github.com/tingly-dev/tingly-box/internal/virtualmodel/openai"
)

// newDefaultRegistries returns Anthropic + OpenAI registries pre-populated
// with their default models.
func newDefaultRegistries() (*anthropicvm.Registry, *openaivm.Registry) {
	a := anthropicvm.NewRegistry()
	anthropicvm.RegisterDefaults(a)
	o := openaivm.NewRegistry()
	openaivm.RegisterDefaults(o)
	return a, o
}

// TestRegisterVModelExtension_RegistersExtension tests that VModel extension is registered
func TestRegisterVModelExtension_RegistersExtension(t *testing.T) {
	extRegistry := NewExtensionRegistry()
	a, o := newDefaultRegistries()

	err := RegisterVModelExtension(extRegistry, a, o)
	if err != nil {
		t.Fatalf("Failed to register VModel extension: %v", err)
	}

	vmodelExt := extRegistry.GetExtension("vmodel")
	if vmodelExt == nil {
		t.Fatal("VModel extension not registered")
	}

	if vmodelExt.ID != "vmodel" {
		t.Errorf("Expected ID 'vmodel', got '%s'", vmodelExt.ID)
	}

	if vmodelExt.Name != "Virtual Models" {
		t.Errorf("Expected Name 'Virtual Models', got '%s'", vmodelExt.Name)
	}

	if vmodelExt.Icon != "memory" {
		t.Errorf("Expected Icon 'memory', got '%s'", vmodelExt.Icon)
	}
}

// TestRegisterVModelExtension_RegistersAllDefaultModels asserts the union of
// both registries (after dedup on shared IDs) covers the expected default models.
func TestRegisterVModelExtension_RegistersAllDefaultModels(t *testing.T) {
	extRegistry := NewExtensionRegistry()
	a, o := newDefaultRegistries()

	err := RegisterVModelExtension(extRegistry, a, o)
	if err != nil {
		t.Fatalf("Failed to register VModel extension: %v", err)
	}

	items := extRegistry.ListItems("vmodel")
	if items == nil {
		t.Fatal("ListItems returned nil")
	}

	// Anthropic registry contributes 10 (claude-3, echo, ask×2, web-search, compact×5).
	// OpenAI registry adds virtual-gpt-4 — the rest dedupe with anthropic.
	// Total: 11.
	if len(items) != 11 {
		t.Errorf("Expected 11 default models, got %d", len(items))
	}

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

	idMap := make(map[string]bool)
	for _, item := range items {
		idMap[item.ID] = true
	}

	for _, expectedID := range expectedIDs {
		if !idMap[expectedID] {
			t.Errorf("Expected model '%s' not registered", expectedID)
		}
	}
}

// TestRegisterVModelExtension_MapsStaticModels tests mapping of static VModels
func TestRegisterVModelExtension_MapsStaticModels(t *testing.T) {
	extRegistry := NewExtensionRegistry()
	a, o := newDefaultRegistries()

	err := RegisterVModelExtension(extRegistry, a, o)
	if err != nil {
		t.Fatalf("Failed to register VModel extension: %v", err)
	}

	item := extRegistry.GetItem("vmodel", "virtual-gpt-4")
	if item == nil {
		t.Fatal("virtual-gpt-4 item not found")
	}

	if item.Type != string(virtualmodel.VirtualModelTypeStatic) {
		t.Errorf("Expected Type 'static', got '%s'", item.Type)
	}

	if item.Name != "Virtual GPT-4" {
		t.Errorf("Expected Name 'Virtual GPT-4', got '%s'", item.Name)
	}

	if item.ExtensionID != "vmodel" {
		t.Errorf("Expected ExtensionID 'vmodel', got '%s'", item.ExtensionID)
	}

	if item.Metadata["provider"] != "openai" {
		t.Errorf("Expected provider 'openai', got %v", item.Metadata["provider"])
	}
}

// TestRegisterVModelExtension_MapsProxyModels tests mapping of proxy VModels
func TestRegisterVModelExtension_MapsProxyModels(t *testing.T) {
	extRegistry := NewExtensionRegistry()
	a, o := newDefaultRegistries()

	err := RegisterVModelExtension(extRegistry, a, o)
	if err != nil {
		t.Fatalf("Failed to register VModel extension: %v", err)
	}

	item := extRegistry.GetItem("vmodel", "compact-thinking")
	if item == nil {
		t.Fatal("compact-thinking item not found")
	}

	if item.Type != string(virtualmodel.VirtualModelTypeProxy) {
		t.Errorf("Expected Type 'proxy', got '%s'", item.Type)
	}

	if item.Name != "Compact Thinking" {
		t.Errorf("Expected Name 'Compact Thinking', got '%s'", item.Name)
	}

	if item.Metadata == nil {
		t.Error("Expected Metadata to be populated")
	}

	if item.Metadata["provider"] != "anthropic" {
		t.Errorf("Expected provider 'anthropic', got %v", item.Metadata["provider"])
	}
}

// TestRegisterVModelExtension_MapsToolModels tests mapping of tool VModels
func TestRegisterVModelExtension_MapsToolModels(t *testing.T) {
	extRegistry := NewExtensionRegistry()
	a, o := newDefaultRegistries()

	err := RegisterVModelExtension(extRegistry, a, o)
	if err != nil {
		t.Fatalf("Failed to register VModel extension: %v", err)
	}

	item := extRegistry.GetItem("vmodel", "ask-user-question")
	if item == nil {
		t.Fatal("ask-user-question item not found")
	}

	if item.Type != string(virtualmodel.VirtualModelTypeTool) {
		t.Errorf("Expected Type 'tool', got '%s'", item.Type)
	}

	if item.Name != "Ask User Question" {
		t.Errorf("Expected Name 'Ask User Question', got '%s'", item.Name)
	}
}

// TestRegisterVModelExtension_PreservesMetadata tests that VModel metadata is preserved
func TestRegisterVModelExtension_PreservesMetadata(t *testing.T) {
	extRegistry := NewExtensionRegistry()
	a, o := newDefaultRegistries()

	err := RegisterVModelExtension(extRegistry, a, o)
	if err != nil {
		t.Fatalf("Failed to register VModel extension: %v", err)
	}

	item := extRegistry.GetItem("vmodel", "compact-thinking")
	if item == nil {
		t.Fatal("compact-thinking item not found")
	}

	if item.Metadata == nil {
		t.Fatal("Expected Metadata to be non-nil for proxy models")
	}

	if _, ok := item.Metadata["vmType"]; !ok {
		t.Error("Expected metadata to contain 'vmType'")
	}

	if _, ok := item.Metadata["provider"]; !ok {
		t.Error("Expected metadata to contain 'provider'")
	}
}

// TestRegisterVModelExtension_EmptyRegistry tests behavior with empty VModel registries
func TestRegisterVModelExtension_EmptyRegistry(t *testing.T) {
	extRegistry := NewExtensionRegistry()
	a := anthropicvm.NewRegistry()
	o := openaivm.NewRegistry()

	err := RegisterVModelExtension(extRegistry, a, o)
	if err != nil {
		t.Fatalf("Failed to register VModel extension: %v", err)
	}

	vmodelExt := extRegistry.GetExtension("vmodel")
	if vmodelExt == nil {
		t.Fatal("VModel extension not registered")
	}

	items := extRegistry.ListItems("vmodel")
	if len(items) != 0 {
		t.Errorf("Expected 0 items for empty registry, got %d", len(items))
	}
}

// TestRegisterVModelExtension_Idempotent tests that calling twice doesn't cause errors
func TestRegisterVModelExtension_Idempotent(t *testing.T) {
	extRegistry := NewExtensionRegistry()
	a, o := newDefaultRegistries()

	err := RegisterVModelExtension(extRegistry, a, o)
	if err != nil {
		t.Fatalf("First registration failed: %v", err)
	}

	items1 := extRegistry.ListItems("vmodel")
	count1 := len(items1)

	_ = RegisterVModelExtension(extRegistry, a, o)

	items2 := extRegistry.ListItems("vmodel")
	count2 := len(items2)

	if count2 > count1 {
		t.Errorf("Item count increased after second registration: %d -> %d", count1, count2)
	}
}

// TestRegisterVModelExtension_DescriptionMapping tests that descriptions are mapped correctly
func TestRegisterVModelExtension_DescriptionMapping(t *testing.T) {
	extRegistry := NewExtensionRegistry()
	a, o := newDefaultRegistries()

	err := RegisterVModelExtension(extRegistry, a, o)
	if err != nil {
		t.Fatalf("Failed to register VModel extension: %v", err)
	}

	tests := []struct {
		itemID       string
		expectedDesc string
	}{
		{
			itemID:       "compact-thinking",
			expectedDesc: "Removes thinking blocks from historical conversation rounds (10-20% compression)",
		},
		{
			itemID:       "compact-round-only",
			expectedDesc: "Keeps only user request + assistant conclusion, removes intermediate process (70-85% compression)",
		},
		{
			itemID:       "virtual-gpt-4",
			expectedDesc: "A virtual model that returns fixed responses for testing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.itemID, func(t *testing.T) {
			item := extRegistry.GetItem("vmodel", tt.itemID)
			if item == nil {
				t.Fatalf("Item '%s' not found", tt.itemID)
			}

			if item.Description != tt.expectedDesc {
				t.Errorf("Description mismatch for '%s': got '%s', want '%s'",
					tt.itemID, item.Description, tt.expectedDesc)
			}
		})
	}
}
