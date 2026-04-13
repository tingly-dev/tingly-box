package extension

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/virtualmodel"
)

// TestRegisterVModelExtension_RegistersExtension tests that VModel extension is registered
func TestRegisterVModelExtension_RegistersExtension(t *testing.T) {
	extRegistry := NewExtensionRegistry()
	vmRegistry := virtualmodel.NewRegistry()

	// Register default VModels
	vmRegistry.RegisterDefaults()

	err := RegisterVModelExtension(extRegistry, vmRegistry)

	if err != nil {
		t.Fatalf("Failed to register VModel extension: %v", err)
	}

	// Verify extension is registered
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

// TestRegisterVModelExtension_RegistersAllDefaultModels tests that all 9 default models are registered
func TestRegisterVModelExtension_RegistersAllDefaultModels(t *testing.T) {
	extRegistry := NewExtensionRegistry()
	vmRegistry := virtualmodel.NewRegistry()

	vmRegistry.RegisterDefaults()

	err := RegisterVModelExtension(extRegistry, vmRegistry)
	if err != nil {
		t.Fatalf("Failed to register VModel extension: %v", err)
	}

	// Get all items for vmodel
	items := extRegistry.ListItems("vmodel")
	if items == nil {
		t.Fatal("ListItems returned nil")
	}

	// Should have 9 default models
	if len(items) != 9 {
		t.Errorf("Expected 9 default models, got %d", len(items))
	}

	// Verify specific models exist
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
	vmRegistry := virtualmodel.NewRegistry()

	vmRegistry.RegisterDefaults()

	err := RegisterVModelExtension(extRegistry, vmRegistry)
	if err != nil {
		t.Fatalf("Failed to register VModel extension: %v", err)
	}

	// Check static model mapping
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
}

// TestRegisterVModelExtension_MapsProxyModels tests mapping of proxy VModels
func TestRegisterVModelExtension_MapsProxyModels(t *testing.T) {
	extRegistry := NewExtensionRegistry()
	vmRegistry := virtualmodel.NewRegistry()

	vmRegistry.RegisterDefaults()

	err := RegisterVModelExtension(extRegistry, vmRegistry)
	if err != nil {
		t.Fatalf("Failed to register VModel extension: %v", err)
	}

	// Check proxy model mapping
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

	// Verify metadata contains transformer info
	if item.Metadata == nil {
		t.Error("Expected Metadata to be populated")
	}
}

// TestRegisterVModelExtension_MapsToolModels tests mapping of tool VModels
func TestRegisterVModelExtension_MapsToolModels(t *testing.T) {
	extRegistry := NewExtensionRegistry()
	vmRegistry := virtualmodel.NewRegistry()

	vmRegistry.RegisterDefaults()

	err := RegisterVModelExtension(extRegistry, vmRegistry)
	if err != nil {
		t.Fatalf("Failed to register VModel extension: %v", err)
	}

	// Check tool model mapping
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
	vmRegistry := virtualmodel.NewRegistry()

	vmRegistry.RegisterDefaults()

	err := RegisterVModelExtension(extRegistry, vmRegistry)
	if err != nil {
		t.Fatalf("Failed to register VModel extension: %v", err)
	}

	// Check proxy model has metadata
	item := extRegistry.GetItem("vmodel", "compact-thinking")
	if item == nil {
		t.Fatal("compact-thinking item not found")
	}

	// Verify metadata contains delegate model info
	if item.Metadata == nil {
		t.Fatal("Expected Metadata to be non-nil for proxy models")
	}

	// Metadata should contain info about the VModel type
	if _, ok := item.Metadata["vmType"]; !ok {
		t.Error("Expected metadata to contain 'vmType'")
	}
}

// TestRegisterVModelExtension_EmptyRegistry tests behavior with empty VModel registry
func TestRegisterVModelExtension_EmptyRegistry(t *testing.T) {
	extRegistry := NewExtensionRegistry()
	vmRegistry := virtualmodel.NewRegistry()

	// Don't register any defaults

	err := RegisterVModelExtension(extRegistry, vmRegistry)
	if err != nil {
		t.Fatalf("Failed to register VModel extension: %v", err)
	}

	// Extension should still be registered
	vmodelExt := extRegistry.GetExtension("vmodel")
	if vmodelExt == nil {
		t.Fatal("VModel extension not registered")
	}

	// But should have no items
	items := extRegistry.ListItems("vmodel")
	if len(items) != 0 {
		t.Errorf("Expected 0 items for empty registry, got %d", len(items))
	}
}

// TestRegisterVModelExtension_Idempotent tests that calling twice doesn't cause errors
func TestRegisterVModelExtension_Idempotent(t *testing.T) {
	extRegistry := NewExtensionRegistry()
	vmRegistry := virtualmodel.NewRegistry()

	vmRegistry.RegisterDefaults()

	// Register once
	err := RegisterVModelExtension(extRegistry, vmRegistry)
	if err != nil {
		t.Fatalf("First registration failed: %v", err)
	}

	// Count items after first registration
	items1 := extRegistry.ListItems("vmodel")
	count1 := len(items1)

	// Register again - should handle gracefully or return error for duplicate
	err = RegisterVModelExtension(extRegistry, vmRegistry)
	// Either success (idempotent) or error (duplicate) is acceptable
	// The key is that it shouldn't panic

	items2 := extRegistry.ListItems("vmodel")
	count2 := len(items2)

	// Item count should not have increased (no duplicates)
	if count2 > count1 {
		t.Errorf("Item count increased after second registration: %d -> %d", count1, count2)
	}
}

// TestRegisterVModelExtension_DescriptionMapping tests that descriptions are mapped correctly
func TestRegisterVModelExtension_DescriptionMapping(t *testing.T) {
	extRegistry := NewExtensionRegistry()
	vmRegistry := virtualmodel.NewRegistry()

	vmRegistry.RegisterDefaults()

	err := RegisterVModelExtension(extRegistry, vmRegistry)
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
