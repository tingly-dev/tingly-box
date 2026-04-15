package extension

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExtensionStore_GetConfig_Exists tests getting existing config
func TestExtensionStore_GetConfig_Exists(t *testing.T) {
	store, tempDir := newTestStore(t)
	defer store.Close()
	defer os.RemoveAll(tempDir)

	// Save a config
	config := &ExtensionConfig{
		ID:      "vmodel",
		Type:    "extension",
		Enabled: true,
		Order:   1,
	}

	err := store.SetConfig(config)
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Retrieve it
	retrieved, err := store.GetConfig("vmodel")
	if err != nil {
		t.Fatalf("Failed to get config: %v", err)
	}

	if retrieved.ID != "vmodel" {
		t.Errorf("Expected ID 'vmodel', got '%s'", retrieved.ID)
	}

	if retrieved.Type != "extension" {
		t.Errorf("Expected Type 'extension', got '%s'", retrieved.Type)
	}

	if retrieved.Enabled != true {
		t.Errorf("Expected Enabled true, got %v", retrieved.Enabled)
	}

	if retrieved.Order != 1 {
		t.Errorf("Expected Order 1, got %d", retrieved.Order)
	}
}

// TestExtensionStore_GetConfig_NotFound tests getting non-existent config
func TestExtensionStore_GetConfig_NotFound(t *testing.T) {
	store, tempDir := newTestStore(t)
	defer store.Close()
	defer os.RemoveAll(tempDir)

	_, err := store.GetConfig("non-existent")

	if err == nil {
		t.Fatal("Expected error for non-existent config, got nil")
	}
}

// TestExtensionStore_SetConfig_Update tests updating existing config
func TestExtensionStore_SetConfig_Update(t *testing.T) {
	store, tempDir := newTestStore(t)
	defer store.Close()
	defer os.RemoveAll(tempDir)

	// Save initial config
	config := &ExtensionConfig{
		ID:      "vmodel",
		Type:    "extension",
		Enabled: true,
		Order:   1,
	}

	err := store.SetConfig(config)
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Update it
	config.Enabled = false
	config.Order = 2

	err = store.SetConfig(config)
	if err != nil {
		t.Fatalf("Failed to update config: %v", err)
	}

	// Verify update
	retrieved, err := store.GetConfig("vmodel")
	if err != nil {
		t.Fatalf("Failed to get config: %v", err)
	}

	if retrieved.Enabled != false {
		t.Errorf("Expected Enabled false after update, got %v", retrieved.Enabled)
	}

	if retrieved.Order != 2 {
		t.Errorf("Expected Order 2 after update, got %d", retrieved.Order)
	}
}

// TestExtensionStore_SetConfig_ItemWithConfig tests saving item with JSON config
func TestExtensionStore_SetConfig_ItemWithConfig(t *testing.T) {
	store, tempDir := newTestStore(t)
	defer store.Close()
	defer os.RemoveAll(tempDir)

	configJSON := `{"delegateModel":"claude-3-5-sonnet-20241022","maxTokens":4096}`

	config := &ExtensionConfig{
		ID:       "compact-thinking",
		Type:     "item",
		ParentID: "vmodel",
		Enabled:  true,
		Config:   configJSON,
		Order:    0,
	}

	err := store.SetConfig(config)
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	retrieved, err := store.GetConfig("compact-thinking")
	if err != nil {
		t.Fatalf("Failed to get config: %v", err)
	}

	if retrieved.Config != configJSON {
		t.Errorf("Config mismatch: got '%s', want '%s'", retrieved.Config, configJSON)
	}
}

// TestExtensionStore_SetConfig_EmptyID tests saving config with empty ID
func TestExtensionStore_SetConfig_EmptyID(t *testing.T) {
	store, tempDir := newTestStore(t)
	defer store.Close()
	defer os.RemoveAll(tempDir)

	config := &ExtensionConfig{
		ID:      "",
		Type:    "extension",
		Enabled: true,
	}

	err := store.SetConfig(config)

	if err == nil {
		t.Fatal("Expected error for empty ID, got nil")
	}
}

// TestExtensionStore_ListConfigs_ReturnsAll tests listing all configs
func TestExtensionStore_ListConfigs_ReturnsAll(t *testing.T) {
	store, tempDir := newTestStore(t)
	defer store.Close()
	defer os.RemoveAll(tempDir)

	// Save multiple configs
	configs := []*ExtensionConfig{
		{ID: "vmodel", Type: "extension", Enabled: true, Order: 1},
		{ID: "mcp", Type: "extension", Enabled: false, Order: 2},
		{ID: "compact-thinking", Type: "item", ParentID: "vmodel", Enabled: true, Order: 0},
	}

	for _, cfg := range configs {
		err := store.SetConfig(cfg)
		if err != nil {
			t.Fatalf("Failed to save config %s: %v", cfg.ID, err)
		}
	}

	// List all
	list, err := store.ListConfigs()
	if err != nil {
		t.Fatalf("Failed to list configs: %v", err)
	}

	if len(list) != 3 {
		t.Errorf("Expected 3 configs, got %d", len(list))
	}

	// Verify all IDs are present
	ids := make(map[string]bool)
	for _, cfg := range list {
		ids[cfg.ID] = true
	}

	for _, expectedID := range []string{"vmodel", "mcp", "compact-thinking"} {
		if !ids[expectedID] {
			t.Errorf("Config '%s' not found in list", expectedID)
		}
	}
}

// TestExtensionStore_ListConfigs_Empty tests listing when store is empty
func TestExtensionStore_ListConfigs_Empty(t *testing.T) {
	store, tempDir := newTestStore(t)
	defer store.Close()
	defer os.RemoveAll(tempDir)

	list, err := store.ListConfigs()

	if err != nil {
		t.Fatalf("Failed to list configs: %v", err)
	}

	if len(list) != 0 {
		t.Errorf("Expected 0 configs, got %d", len(list))
	}
}

// TestExtensionStore_DeleteConfig tests deleting a config
func TestExtensionStore_DeleteConfig(t *testing.T) {
	store, tempDir := newTestStore(t)
	defer store.Close()
	defer os.RemoveAll(tempDir)

	// Save a config
	config := &ExtensionConfig{
		ID:      "vmodel",
		Type:    "extension",
		Enabled: true,
	}

	err := store.SetConfig(config)
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Verify it exists
	_, err = store.GetConfig("vmodel")
	if err != nil {
		t.Fatal("Config should exist before deletion")
	}

	// Delete it
	err = store.DeleteConfig("vmodel")
	if err != nil {
		t.Fatalf("Failed to delete config: %v", err)
	}

	// Verify it's gone
	_, err = store.GetConfig("vmodel")
	if err == nil {
		t.Fatal("Config should not exist after deletion")
	}
}

// TestExtensionStore_DeleteConfig_NotFound tests deleting non-existent config
func TestExtensionStore_DeleteConfig_NotFound(t *testing.T) {
	store, tempDir := newTestStore(t)
	defer store.Close()
	defer os.RemoveAll(tempDir)

	err := store.DeleteConfig("non-existent")

	if err == nil {
		t.Fatal("Expected error for deleting non-existent config, got nil")
	}
}

// TestExtensionStore_GetConfigsByParent tests getting configs by parent ID
func TestExtensionStore_GetConfigsByParent(t *testing.T) {
	store, tempDir := newTestStore(t)
	defer store.Close()
	defer os.RemoveAll(tempDir)

	// Save configs with different parents
	configs := []*ExtensionConfig{
		{ID: "item1", Type: "item", ParentID: "vmodel", Enabled: true, Order: 1},
		{ID: "item2", Type: "item", ParentID: "vmodel", Enabled: false, Order: 2},
		{ID: "item3", Type: "item", ParentID: "mcp", Enabled: true, Order: 0},
	}

	for _, cfg := range configs {
		err := store.SetConfig(cfg)
		if err != nil {
			t.Fatalf("Failed to save config: %v", err)
		}
	}

	// Get vmodel items
	vmodelItems, err := store.GetConfigsByParent("vmodel")
	if err != nil {
		t.Fatalf("Failed to get configs by parent: %v", err)
	}

	if len(vmodelItems) != 2 {
		t.Errorf("Expected 2 items for vmodel, got %d", len(vmodelItems))
	}

	// Verify IDs
	ids := make(map[string]bool)
	for _, cfg := range vmodelItems {
		ids[cfg.ID] = true
	}

	if !ids["item1"] || !ids["item2"] {
		t.Error("Not all vmodel items were returned")
	}

	if ids["item3"] {
		t.Error("mcp item should not be in vmodel items")
	}
}

// TestExtensionStore_GetConfigsByType tests getting configs by type
func TestExtensionStore_GetConfigsByType(t *testing.T) {
	store, tempDir := newTestStore(t)
	defer store.Close()
	defer os.RemoveAll(tempDir)

	// Save configs with different types
	configs := []*ExtensionConfig{
		{ID: "vmodel", Type: "extension", Enabled: true, Order: 1},
		{ID: "mcp", Type: "extension", Enabled: false, Order: 2},
		{ID: "item1", Type: "item", ParentID: "vmodel", Enabled: true, Order: 0},
	}

	for _, cfg := range configs {
		err := store.SetConfig(cfg)
		if err != nil {
			t.Fatalf("Failed to save config: %v", err)
		}
	}

	// Get extension configs
	extensions, err := store.GetConfigsByType("extension")
	if err != nil {
		t.Fatalf("Failed to get configs by type: %v", err)
	}

	if len(extensions) != 2 {
		t.Errorf("Expected 2 extension configs, got %d", len(extensions))
	}

	// Get item configs
	items, err := store.GetConfigsByType("item")
	if err != nil {
		t.Fatalf("Failed to get configs by type: %v", err)
	}

	if len(items) != 1 {
		t.Errorf("Expected 1 item config, got %d", len(items))
	}
}

// TestExtensionStore_ConcurrentUpdates tests concurrent updates are safe
func TestExtensionStore_ConcurrentUpdates(t *testing.T) {
	store, tempDir := newTestStore(t)
	defer store.Close()
	defer os.RemoveAll(tempDir)

	config := &ExtensionConfig{
		ID:      "vmodel",
		Type:    "extension",
		Enabled: true,
	}

	done := make(chan bool)

	// Perform 100 concurrent updates
	for i := 0; i < 100; i++ {
		go func(idx int) {
			cfg := *config
			cfg.Order = idx
			_ = store.SetConfig(&cfg)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Verify final state exists
	_, err := store.GetConfig("vmodel")
	if err != nil {
		t.Errorf("Config should exist after concurrent updates: %v", err)
	}
}

// newTestStore creates a test store with a temporary database
func newTestStore(t *testing.T) (*ExtensionStore, string) {
	t.Helper()

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "extension-store-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create database
	dbPath := filepath.Join(tempDir, "test.db")

	store, err := NewExtensionStore(dbPath)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create store: %v", err)
	}

	return store, tempDir
}
