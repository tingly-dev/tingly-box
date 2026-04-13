package extension

import (
	"testing"
)

// TestExtensionRegistry_RegisterExtension_Success tests registering a new extension
func TestExtensionRegistry_RegisterExtension_Success(t *testing.T) {
	registry := NewExtensionRegistry()

	ext := &Extension{
		ID:          "vmodel",
		Name:        "Virtual Models",
		Description: "Virtual models for testing",
		Icon:        "memory",
	}

	err := registry.RegisterExtension(ext)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	retrieved := registry.GetExtension("vmodel")
	if retrieved == nil {
		t.Fatal("Extension not found after registration")
	}

	if retrieved.ID != "vmodel" {
		t.Errorf("Expected ID 'vmodel', got '%s'", retrieved.ID)
	}

	if retrieved.Name != "Virtual Models" {
		t.Errorf("Expected Name 'Virtual Models', got '%s'", retrieved.Name)
	}
}

// TestExtensionRegistry_RegisterExtension_Duplicate tests duplicate extension registration
func TestExtensionRegistry_RegisterExtension_Duplicate(t *testing.T) {
	registry := NewExtensionRegistry()

	ext1 := &Extension{
		ID:   "vmodel",
		Name: "First VModel",
	}

	ext2 := &Extension{
		ID:   "vmodel",
		Name: "Duplicate VModel",
	}

	_ = registry.RegisterExtension(ext1)
	err := registry.RegisterExtension(ext2)

	if err == nil {
		t.Fatal("Expected error for duplicate registration, got nil")
	}

	// Verify first extension is still registered
	retrieved := registry.GetExtension("vmodel")
	if retrieved.Name != "First VModel" {
		t.Errorf("Original extension was overwritten")
	}
}

// TestExtensionRegistry_RegisterItem_Success tests registering a new item
func TestExtensionRegistry_RegisterItem_Success(t *testing.T) {
	registry := NewExtensionRegistry()

	// First register the extension
	ext := &Extension{
		ID:   "vmodel",
		Name: "Virtual Models",
	}
	_ = registry.RegisterExtension(ext)

	// Then register an item
	item := &ExtensionItem{
		ID:          "compact-thinking",
		ExtensionID: "vmodel",
		Name:        "Compact Thinking",
		Description: "Removes thinking blocks",
		Type:        "proxy",
	}

	err := registry.RegisterItem(item)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	retrieved := registry.GetItem("vmodel", "compact-thinking")
	if retrieved == nil {
		t.Fatal("Item not found after registration")
	}

	if retrieved.ID != "compact-thinking" {
		t.Errorf("Expected ID 'compact-thinking', got '%s'", retrieved.ID)
	}

	if retrieved.ExtensionID != "vmodel" {
		t.Errorf("Expected ExtensionID 'vmodel', got '%s'", retrieved.ExtensionID)
	}
}

// TestExtensionRegistry_RegisterItem_ExtensionNotFound tests registering item for non-existent extension
func TestExtensionRegistry_RegisterItem_ExtensionNotFound(t *testing.T) {
	registry := NewExtensionRegistry()

	item := &ExtensionItem{
		ID:          "compact-thinking",
		ExtensionID: "non-existent",
		Name:        "Compact Thinking",
	}

	err := registry.RegisterItem(item)

	if err == nil {
		t.Fatal("Expected error for non-existent extension, got nil")
	}
}

// TestExtensionRegistry_RegisterItem_Duplicate tests duplicate item registration
func TestExtensionRegistry_RegisterItem_Duplicate(t *testing.T) {
	registry := NewExtensionRegistry()

	ext := &Extension{ID: "vmodel", Name: "VModel"}
	_ = registry.RegisterExtension(ext)

	item1 := &ExtensionItem{
		ID:          "compact-thinking",
		ExtensionID: "vmodel",
		Name:        "First",
	}

	item2 := &ExtensionItem{
		ID:          "compact-thinking",
		ExtensionID: "vmodel",
		Name:        "Duplicate",
	}

	_ = registry.RegisterItem(item1)
	err := registry.RegisterItem(item2)

	if err == nil {
		t.Fatal("Expected error for duplicate item registration, got nil")
	}

	// Verify first item is still registered
	retrieved := registry.GetItem("vmodel", "compact-thinking")
	if retrieved.Name != "First" {
		t.Errorf("Original item was overwritten")
	}
}

// TestExtensionRegistry_GetExtension_NotFound tests getting non-existent extension
func TestExtensionRegistry_GetExtension_NotFound(t *testing.T) {
	registry := NewExtensionRegistry()

	retrieved := registry.GetExtension("non-existent")

	if retrieved != nil {
		t.Errorf("Expected nil for non-existent extension, got %v", retrieved)
	}
}

// TestExtensionRegistry_GetItem_NotFound tests getting non-existent item
func TestExtensionRegistry_GetItem_NotFound(t *testing.T) {
	registry := NewExtensionRegistry()

	// Test with non-existent extension
	retrieved := registry.GetItem("non-existent", "item-id")
	if retrieved != nil {
		t.Errorf("Expected nil for non-existent extension, got %v", retrieved)
	}

	// Test with existing extension but non-existent item
	ext := &Extension{ID: "vmodel", Name: "VModel"}
	_ = registry.RegisterExtension(ext)

	retrieved = registry.GetItem("vmodel", "non-existent")
	if retrieved != nil {
		t.Errorf("Expected nil for non-existent item, got %v", retrieved)
	}
}

// TestExtensionRegistry_ListExtensions_ReturnsAll tests listing all extensions
func TestExtensionRegistry_ListExtensions_ReturnsAll(t *testing.T) {
	registry := NewExtensionRegistry()

	// Register multiple extensions
	ext1 := &Extension{ID: "vmodel", Name: "Virtual Models"}
	ext2 := &Extension{ID: "mcp", Name: "MCP"}
	ext3 := &Extension{ID: "future", Name: "Future"}

	_ = registry.RegisterExtension(ext1)
	_ = registry.RegisterExtension(ext2)
	_ = registry.RegisterExtension(ext3)

	extensions := registry.ListExtensions()

	if len(extensions) != 3 {
		t.Errorf("Expected 3 extensions, got %d", len(extensions))
	}

	// Verify all IDs are present
	ids := make(map[string]bool)
	for _, ext := range extensions {
		ids[ext.ID] = true
	}

	if !ids["vmodel"] || !ids["mcp"] || !ids["future"] {
		t.Error("Not all extensions were returned")
	}
}

// TestExtensionRegistry_ListExtensions_Empty tests listing when registry is empty
func TestExtensionRegistry_ListExtensions_Empty(t *testing.T) {
	registry := NewExtensionRegistry()

	extensions := registry.ListExtensions()

	if len(extensions) != 0 {
		t.Errorf("Expected 0 extensions, got %d", len(extensions))
	}
}

// TestExtensionRegistry_ListItems_ReturnsAllForExtension tests listing items for an extension
func TestExtensionRegistry_ListItems_ReturnsAllForExtension(t *testing.T) {
	registry := NewExtensionRegistry()

	ext := &Extension{ID: "vmodel", Name: "VModel"}
	_ = registry.RegisterExtension(ext)

	// Register multiple items for the same extension
	item1 := &ExtensionItem{ID: "item1", ExtensionID: "vmodel", Name: "Item 1"}
	item2 := &ExtensionItem{ID: "item2", ExtensionID: "vmodel", Name: "Item 2"}
	item3 := &ExtensionItem{ID: "item3", ExtensionID: "vmodel", Name: "Item 3"}

	_ = registry.RegisterItem(item1)
	_ = registry.RegisterItem(item2)
	_ = registry.RegisterItem(item3)

	items := registry.ListItems("vmodel")

	if len(items) != 3 {
		t.Errorf("Expected 3 items, got %d", len(items))
	}

	// Verify all IDs are present
	ids := make(map[string]bool)
	for _, item := range items {
		ids[item.ID] = true
	}

	if !ids["item1"] || !ids["item2"] || !ids["item3"] {
		t.Error("Not all items were returned")
	}
}

// TestExtensionRegistry_ListItems_EmptyForExtension tests listing items when extension has no items
func TestExtensionRegistry_ListItems_EmptyForExtension(t *testing.T) {
	registry := NewExtensionRegistry()

	ext := &Extension{ID: "vmodel", Name: "VModel"}
	_ = registry.RegisterExtension(ext)

	items := registry.ListItems("vmodel")

	if len(items) != 0 {
		t.Errorf("Expected 0 items, got %d", len(items))
	}
}

// TestExtensionRegistry_ListItems_ExtensionNotFound tests listing items for non-existent extension
func TestExtensionRegistry_ListItems_ExtensionNotFound(t *testing.T) {
	registry := NewExtensionRegistry()

	items := registry.ListItems("non-existent")

	if items != nil {
		t.Errorf("Expected nil for non-existent extension, got %v", items)
	}
}

// TestExtensionRegistry_ConcurrentRegistration tests concurrent registration is safe
func TestExtensionRegistry_ConcurrentRegistration(t *testing.T) {
	registry := NewExtensionRegistry()

	done := make(chan bool)

	// Register 100 extensions concurrently
	for i := 0; i < 100; i++ {
		go func(idx int) {
			ext := &Extension{
				ID:   "ext-" + string(rune(idx)),
				Name: "Extension",
			}
			_ = registry.RegisterExtension(ext)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Verify count (may vary due to duplicates, but should not panic)
	extensions := registry.ListExtensions()
	if extensions == nil {
		t.Error("ListExtensions returned nil, expected slice")
	}
}
