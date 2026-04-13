package extension

import (
	"os"
	"testing"
)

// TestExtensionService_ListExtensions_ReturnsAll tests listing all extensions
func TestExtensionService_ListExtensions_ReturnsAll(t *testing.T) {
	service := newTestService(t)
	defer service.Close()

	// Register extensions
	registerTestExtensions(t, service)

	// List extensions
	views, err := service.ListExtensions()

	if err != nil {
		t.Fatalf("Failed to list extensions: %v", err)
	}

	if len(views) != 2 {
		t.Errorf("Expected 2 extensions, got %d", len(views))
	}

	// Verify vmodel extension
	var vmodelView *ExtensionView
	for _, v := range views {
		if v.ID == "vmodel" {
			vmodelView = v
			break
		}
	}

	if vmodelView == nil {
		t.Fatal("vmodel extension not found")
	}

	// Verify default state (no DB config, should use defaults)
	if !vmodelView.Enabled {
		t.Error("Expected vmodel to be enabled by default")
	}

	// Verify items are included
	if len(vmodelView.Items) != 2 {
		t.Errorf("Expected 2 items for vmodel, got %d", len(vmodelView.Items))
	}
}

// TestExtensionService_ListExtensions_WithDatabaseConfig tests merging with database config
func TestExtensionService_ListExtensions_WithDatabaseConfig(t *testing.T) {
	service := newTestService(t)
	defer service.Close()

	// Register extensions
	registerTestExtensions(t, service)

	// Save database config to disable vmodel
	config := &ExtensionConfig{
		ID:      "vmodel",
		Type:    "extension",
		Enabled: false,
		Order:   5,
	}

	err := service.store.SetConfig(config)
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// List extensions
	views, err := service.ListExtensions()

	if err != nil {
		t.Fatalf("Failed to list extensions: %v", err)
	}

	// Find vmodel
	var vmodelView *ExtensionView
	for _, v := range views {
		if v.ID == "vmodel" {
			vmodelView = v
			break
		}
	}

	if vmodelView == nil {
		t.Fatal("vmodel extension not found")
	}

	// Verify database state is merged
	if vmodelView.Enabled {
		t.Error("Expected vmodel to be disabled (from DB config)")
	}

	if vmodelView.Order != 5 {
		t.Errorf("Expected Order 5 (from DB), got %d", vmodelView.Order)
	}
}

// TestExtensionService_GetExtension_Exists tests getting a specific extension
func TestExtensionService_GetExtension_Exists(t *testing.T) {
	service := newTestService(t)
	defer service.Close()

	registerTestExtensions(t, service)

	view, err := service.GetExtension("vmodel")

	if err != nil {
		t.Fatalf("Failed to get extension: %v", err)
	}

	if view.ID != "vmodel" {
		t.Errorf("Expected ID 'vmodel', got '%s'", view.ID)
	}

	if view.Name != "Virtual Models" {
		t.Errorf("Expected Name 'Virtual Models', got '%s'", view.Name)
	}

	if len(view.Items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(view.Items))
	}
}

// TestExtensionService_GetExtension_NotFound tests getting non-existent extension
func TestExtensionService_GetExtension_NotFound(t *testing.T) {
	service := newTestService(t)
	defer service.Close()

	_, err := service.GetExtension("non-existent")

	if err == nil {
		t.Fatal("Expected error for non-existent extension, got nil")
	}
}

// TestExtensionService_UpdateExtension_Enabled tests updating extension enabled state
func TestExtensionService_UpdateExtension_Enabled(t *testing.T) {
	service := newTestService(t)
	defer service.Close()

	registerTestExtensions(t, service)

	// Disable vmodel
	enabled := false
	err := service.UpdateExtension("vmodel", &enabled, nil)

	if err != nil {
		t.Fatalf("Failed to update extension: %v", err)
	}

	// Verify update
	view, err := service.GetExtension("vmodel")
	if err != nil {
		t.Fatalf("Failed to get extension: %v", err)
	}

	if view.Enabled {
		t.Error("Expected extension to be disabled")
	}

	// Verify it persists in database
	config, err := service.store.GetConfig("vmodel")
	if err != nil {
		t.Fatalf("Failed to get config from store: %v", err)
	}

	if config.Enabled {
		t.Error("Expected database config to have Enabled=false")
	}
}

// TestExtensionService_UpdateExtension_Order tests updating extension order
func TestExtensionService_UpdateExtension_Order(t *testing.T) {
	service := newTestService(t)
	defer service.Close()

	registerTestExtensions(t, service)

	// Set order
	order := 10
	err := service.UpdateExtension("vmodel", nil, &order)

	if err != nil {
		t.Fatalf("Failed to update extension: %v", err)
	}

	// Verify update
	view, err := service.GetExtension("vmodel")
	if err != nil {
		t.Fatalf("Failed to get extension: %v", err)
	}

	if view.Order != 10 {
		t.Errorf("Expected Order 10, got %d", view.Order)
	}
}

// TestExtensionService_UpdateExtension_NotFound tests updating non-existent extension
func TestExtensionService_UpdateExtension_NotFound(t *testing.T) {
	service := newTestService(t)
	defer service.Close()

	enabled := false
	err := service.UpdateExtension("non-existent", &enabled, nil)

	if err == nil {
		t.Fatal("Expected error for non-existent extension, got nil")
	}
}

// TestExtensionService_GetItem_Exists tests getting a specific item
func TestExtensionService_GetItem_Exists(t *testing.T) {
	service := newTestService(t)
	defer service.Close()

	registerTestExtensions(t, service)

	item, err := service.GetItem("vmodel", "compact-thinking")

	if err != nil {
		t.Fatalf("Failed to get item: %v", err)
	}

	if item.ID != "compact-thinking" {
		t.Errorf("Expected ID 'compact-thinking', got '%s'", item.ID)
	}

	if item.ExtensionID != "vmodel" {
		t.Errorf("Expected ExtensionID 'vmodel', got '%s'", item.ExtensionID)
	}

	if item.Name != "Compact Thinking" {
		t.Errorf("Expected Name 'Compact Thinking', got '%s'", item.Name)
	}

	// Verify default enabled state
	if !item.Enabled {
		t.Error("Expected item to be enabled by default")
	}
}

// TestExtensionService_GetItem_ExtensionNotFound tests getting item for non-existent extension
func TestExtensionService_GetItem_ExtensionNotFound(t *testing.T) {
	service := newTestService(t)
	defer service.Close()

	_, err := service.GetItem("non-existent", "item-id")

	if err == nil {
		t.Fatal("Expected error for non-existent extension, got nil")
	}
}

// TestExtensionService_GetItem_ItemNotFound tests getting non-existent item
func TestExtensionService_GetItem_ItemNotFound(t *testing.T) {
	service := newTestService(t)
	defer service.Close()

	registerTestExtensions(t, service)

	_, err := service.GetItem("vmodel", "non-existent")

	if err == nil {
		t.Fatal("Expected error for non-existent item, got nil")
	}
}

// TestExtensionService_UpdateItem_Enabled tests updating item enabled state
func TestExtensionService_UpdateItem_Enabled(t *testing.T) {
	service := newTestService(t)
	defer service.Close()

	registerTestExtensions(t, service)

	// Disable item
	enabled := false
	err := service.UpdateItem("vmodel", "compact-thinking", &enabled, nil)

	if err != nil {
		t.Fatalf("Failed to update item: %v", err)
	}

	// Verify update
	item, err := service.GetItem("vmodel", "compact-thinking")
	if err != nil {
		t.Fatalf("Failed to get item: %v", err)
	}

	if item.Enabled {
		t.Error("Expected item to be disabled")
	}
}

// TestExtensionService_UpdateItem_Config tests updating item config
func TestExtensionService_UpdateItem_Config(t *testing.T) {
	service := newTestService(t)
	defer service.Close()

	registerTestExtensions(t, service)

	// Update config
	configJSON := `{"delegateModel":"claude-3-5-sonnet-20241022"}`
	err := service.UpdateItem("vmodel", "compact-thinking", nil, &configJSON)

	if err != nil {
		t.Fatalf("Failed to update item: %v", err)
	}

	// Verify update
	item, err := service.GetItem("vmodel", "compact-thinking")
	if err != nil {
		t.Fatalf("Failed to get item: %v", err)
	}

	if item.Config != configJSON {
		t.Errorf("Expected Config '%s', got '%s'", configJSON, item.Config)
	}
}

// TestExtensionService_UpdateItem_NotFound tests updating non-existent item
func TestExtensionService_UpdateItem_NotFound(t *testing.T) {
	service := newTestService(t)
	defer service.Close()

	registerTestExtensions(t, service)

	enabled := false
	err := service.UpdateItem("vmodel", "non-existent", &enabled, nil)

	if err == nil {
		t.Fatal("Expected error for non-existent item, got nil")
	}
}

// TestExtensionService_MergeWithDefaults tests merging registry data with default values
func TestExtensionService_MergeWithDefaults(t *testing.T) {
	service := newTestService(t)
	defer service.Close()

	// Register extension without database config
	ext := &Extension{
		ID:          "vmodel",
		Name:        "Virtual Models",
		Description: "Virtual models for testing",
		Icon:        "memory",
	}
	_ = service.registry.RegisterExtension(ext)

	// Get extension - should use defaults
	view, err := service.GetExtension("vmodel")
	if err != nil {
		t.Fatalf("Failed to get extension: %v", err)
	}

	// Verify default values
	if !view.Enabled {
		t.Error("Expected default Enabled=true")
	}

	if view.Order != 0 {
		t.Error("Expected default Order=0")
	}
}

// TestExtensionService_ListItemsByExtension tests listing items for a specific extension
func TestExtensionService_ListItemsByExtension(t *testing.T) {
	service := newTestService(t)
	defer service.Close()

	registerTestExtensions(t, service)

	items, err := service.ListItemsByExtension("vmodel")
	if err != nil {
		t.Fatalf("Failed to list items: %v", err)
	}

	if len(items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(items))
	}

	// Verify item IDs
	ids := make(map[string]bool)
	for _, item := range items {
		ids[item.ID] = true
	}

	if !ids["compact-thinking"] || !ids["compact-round-only"] {
		t.Error("Not all expected items were returned")
	}
}

// newTestService creates a test service with temporary database
func newTestService(t *testing.T) *ExtensionService {
	t.Helper()

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "extension-service-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create store
	store, err := NewExtensionStore(tempDir + "/test.db")
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create store: %v", err)
	}

	// Create registry
	registry := NewExtensionRegistry()

	// Create service
	return NewExtensionService(registry, store)
}

// registerTestExtensions registers test extensions and items
func registerTestExtensions(t *testing.T, service *ExtensionService) {
	t.Helper()

	// Register vmodel extension
	ext := &Extension{
		ID:          "vmodel",
		Name:        "Virtual Models",
		Description: "Virtual models for testing",
		Icon:        "memory",
	}
	err := service.registry.RegisterExtension(ext)
	if err != nil {
		t.Fatalf("Failed to register extension: %v", err)
	}

	// Register mcp extension
	mcpExt := &Extension{
		ID:          "mcp",
		Name:        "MCP",
		Description: "Model Context Protocol",
		Icon:        "extension",
	}
	err = service.registry.RegisterExtension(mcpExt)
	if err != nil {
		t.Fatalf("Failed to register extension: %v", err)
	}

	// Register items for vmodel
	item1 := &ExtensionItem{
		ID:          "compact-thinking",
		ExtensionID: "vmodel",
		Name:        "Compact Thinking",
		Description: "Removes thinking blocks",
		Type:        "proxy",
		Metadata: map[string]interface{}{
			"transformer": "smart-compact",
		},
	}
	err = service.registry.RegisterItem(item1)
	if err != nil {
		t.Fatalf("Failed to register item: %v", err)
	}

	item2 := &ExtensionItem{
		ID:          "compact-round-only",
		ExtensionID: "vmodel",
		Name:        "Compact Round Only",
		Description: "Keeps only user request + assistant conclusion",
		Type:        "proxy",
	}
	err = service.registry.RegisterItem(item2)
	if err != nil {
		t.Fatalf("Failed to register item: %v", err)
	}
}
