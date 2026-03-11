package db

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestNewStoreManager tests creating a new StoreManager.
func TestNewStoreManager(t *testing.T) {
	tempDir := t.TempDir()

	sm, err := NewStoreManager(tempDir)
	if err != nil {
		t.Fatalf("NewStoreManager() failed: %v", err)
	}
	defer sm.Close()

	if sm == nil {
		t.Fatal("NewStoreManager() returned nil")
	}

	if sm.BaseDir() != tempDir {
		t.Errorf("BaseDir() = %s, want %s", sm.BaseDir(), tempDir)
	}
}

// TestNewStoreManagerWithCustomConfig tests creating a StoreManager with custom config.
func TestNewStoreManagerWithCustomConfig(t *testing.T) {
	tempDir := t.TempDir()

	config := StoreManagerConfig{
		BaseDir:     tempDir,
		BusyTimeout: 10000,
	}

	sm, err := NewStoreManagerWithConfig(config)
	if err != nil {
		t.Fatalf("NewStoreManagerWithConfig() failed: %v", err)
	}
	defer sm.Close()

	if sm == nil {
		t.Fatal("NewStoreManagerWithConfig() returned nil")
	}
}

// TestNewStoreManagerEmptyBaseDir tests that empty base directory returns error.
func TestNewStoreManagerEmptyBaseDir(t *testing.T) {
	sm, err := NewStoreManager("")
	if err == nil {
		sm.Close()
		t.Fatal("NewStoreManager() with empty baseDir should return error")
	}
	if sm != nil {
		t.Error("NewStoreManager() should return nil StoreManager on error")
	}
}

// TestNewStoreManagerCreatesDirectory tests that StoreManager creates the base directory.
func TestNewStoreManagerCreatesDirectory(t *testing.T) {
	tempDir := t.TempDir()
	baseDir := filepath.Join(tempDir, "nested", "dir", "that", "does", "not", "exist")

	// Verify directory doesn't exist
	if _, err := os.Stat(baseDir); !os.IsNotExist(err) {
		t.Fatalf("Test setup failed: directory should not exist")
	}

	sm, err := NewStoreManager(baseDir)
	if err != nil {
		t.Fatalf("NewStoreManager() failed: %v", err)
	}
	defer sm.Close()

	// Verify directory was created
	if info, err := os.Stat(baseDir); err != nil {
		t.Errorf("Directory was not created: %v", err)
	} else if !info.IsDir() {
		t.Error("Path is not a directory")
	}

	// Verify DB file was created
	dbPath := constant.GetDBFile(baseDir)
	if info, err := os.Stat(dbPath); err != nil {
		t.Errorf("Database file was not created: %v", err)
	} else if info.IsDir() {
		t.Error("DB path is a directory, not a file")
	}
}

// TestStoreManager_Accessors tests that all accessor methods return non-nil stores.
func TestStoreManager_Accessors(t *testing.T) {
	tempDir := t.TempDir()

	sm, err := NewStoreManager(tempDir)
	if err != nil {
		t.Fatalf("NewStoreManager() failed: %v", err)
	}
	defer sm.Close()

	tests := []struct {
		name string
		fn   func() interface{}
	}{
		{"Stats", func() interface{} { return sm.Stats() }},
		{"Usage", func() interface{} { return sm.Usage() }},
		{"RuleState", func() interface{} { return sm.RuleState() }},
		{"Provider", func() interface{} { return sm.Provider() }},
		{"ToolConfig", func() interface{} { return sm.ToolConfig() }},
		{"ImBotSettings", func() interface{} { return sm.ImBotSettings() }},
		{"ModelCapability", func() interface{} { return sm.ModelCapability() }},
		{"Model", func() interface{} { return sm.Model() }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := tt.fn()
			if store == nil {
				t.Errorf("%s() returned nil", tt.name)
			}
		})
	}
}

// TestStoreManager_Close tests closing the StoreManager.
func TestStoreManager_Close(t *testing.T) {
	tempDir := t.TempDir()

	sm, err := NewStoreManager(tempDir)
	if err != nil {
		t.Fatalf("NewStoreManager() failed: %v", err)
	}

	// Close the manager
	if err := sm.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}

	// Verify all accessors return nil after close
	if sm.Stats() != nil {
		t.Error("Stats() should return nil after Close()")
	}
	if sm.Usage() != nil {
		t.Error("Usage() should return nil after Close()")
	}
	if sm.RuleState() != nil {
		t.Error("RuleState() should return nil after Close()")
	}
	if sm.Provider() != nil {
		t.Error("Provider() should return nil after Close()")
	}
	if sm.ToolConfig() != nil {
		t.Error("ToolConfig() should return nil after Close()")
	}
	if sm.ImBotSettings() != nil {
		t.Error("ImBotSettings() should return nil after Close()")
	}
	if sm.ModelCapability() != nil {
		t.Error("ModelCapability() should return nil after Close()")
	}
	if sm.Model() != nil {
		t.Error("Model() should return nil after Close()")
	}

	// Double close should not error
	if err := sm.Close(); err != nil {
		t.Errorf("Second Close() returned error: %v", err)
	}
}

// TestStoreManager_HealthCheck tests the health check functionality.
func TestStoreManager_HealthCheck(t *testing.T) {
	tempDir := t.TempDir()

	sm, err := NewStoreManager(tempDir)
	if err != nil {
		t.Fatalf("NewStoreManager() failed: %v", err)
	}
	defer sm.Close()

	status, err := sm.HealthCheck()
	if err != nil {
		t.Fatalf("HealthCheck() failed: %v", err)
	}

	if !status.Healthy {
		t.Errorf("HealthCheck() returned unhealthy: %+v", status)
	}

	if status.TotalStores != 8 {
		t.Errorf("TotalStores = %d, want 8", status.TotalStores)
	}

	if status.HealthyStores != 8 {
		t.Errorf("HealthyStores = %d, want 8", status.HealthyStores)
	}

	if status.UnhealthyStores != 0 {
		t.Errorf("UnhealthyStores = %d, want 0", status.UnhealthyStores)
	}

	expectedStores := []string{
		"stats", "usage", "ruleState", "provider",
		"toolConfig", "imbotSettings", "modelCapability", "model",
	}
	for _, name := range expectedStores {
		if status.StoreStatus[name] != HealthStatusOK {
			t.Errorf("Store %s status = %s, want %s", name, status.StoreStatus[name], HealthStatusOK)
		}
	}
}

// TestStoreManager_HealthCheckAfterClose tests health check after close.
func TestStoreManager_HealthCheckAfterClose(t *testing.T) {
	tempDir := t.TempDir()

	sm, err := NewStoreManager(tempDir)
	if err != nil {
		t.Fatalf("NewStoreManager() failed: %v", err)
	}

	sm.Close()

	status, err := sm.HealthCheck()
	if err != nil {
		t.Fatalf("HealthCheck() failed: %v", err)
	}

	if status.Healthy {
		t.Error("HealthCheck() should return unhealthy after Close()")
	}

	if status.UnhealthyStores != 8 {
		t.Errorf("UnhealthyStores = %d, want 8", status.UnhealthyStores)
	}
}

// TestStoreManager_StoreOperations tests that stores can perform actual operations.
func TestStoreManager_StoreOperations(t *testing.T) {
	tempDir := t.TempDir()

	sm, err := NewStoreManager(tempDir)
	if err != nil {
		t.Fatalf("NewStoreManager() failed: %v", err)
	}
	defer sm.Close()

	// Test ProviderStore operations
	providerStore := sm.Provider()

	// Create a test provider
	provider := &typ.Provider{
		UUID:          "test-provider-1",
		Name:          "Test Provider",
		APIBase:       "https://api.example.com",
		APIStyle:      "openai",
		AuthType:      "api_key",
		Token:         "test-token",
		Enabled:       true,
		NoKeyRequired: false,
	}

	if err := providerStore.Save(provider); err != nil {
		t.Fatalf("Failed to save provider: %v", err)
	}

	// Retrieve the provider
	retrieved, err := providerStore.GetByUUID("test-provider-1")
	if err != nil {
		t.Fatalf("Failed to get provider: %v", err)
	}

	if retrieved.Name != "Test Provider" {
		t.Errorf("Provider name = %s, want 'Test Provider'", retrieved.Name)
	}

	// Test UsageStore operations
	usageStore := sm.Usage()

	// Record usage
	record := &UsageRecord{
		ProviderUUID: "test-provider-1",
		ProviderName: "Test Provider",
		Model:        "gpt-4",
		Scenario:     "openai",
		Timestamp:    time.Now(),
		InputTokens:  100,
		OutputTokens: 50,
		TotalTokens:  150,
		Status:       "success",
	}

	if err := usageStore.RecordUsage(record); err != nil {
		t.Fatalf("Failed to record usage: %v", err)
	}
}

// TestStoreManager_ConcurrentAccess tests concurrent access to stores.
func TestStoreManager_ConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()

	sm, err := NewStoreManager(tempDir)
	if err != nil {
		t.Fatalf("NewStoreManager() failed: %v", err)
	}
	defer sm.Close()

	const numGoroutines = 100
	const iterationsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterationsPerGoroutine; j++ {
				// Access various stores concurrently
				_ = sm.Stats()
				_ = sm.Usage()
				_ = sm.Provider()
				_ = sm.RuleState()
				_ = sm.ToolConfig()
				_ = sm.ImBotSettings()
				_ = sm.ModelCapability()
				_ = sm.Model()
			}
		}()
	}

	wg.Wait()
	// If we get here without panic or deadlock, the test passes
}

// TestStoreManager_ThreadSafety tests thread safety with mixed reads and writes.
func TestStoreManager_ThreadSafety(t *testing.T) {
	tempDir := t.TempDir()

	sm, err := NewStoreManager(tempDir)
	if err != nil {
		t.Fatalf("NewStoreManager() failed: %v", err)
	}
	defer sm.Close()

	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// Half reading, half writing health checks
	for i := 0; i < numGoroutines; i++ {
		// Reader goroutine
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = sm.Stats()
				_ = sm.Usage()
				_ = sm.Provider()
			}
		}()

		// Writer/health check goroutine
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_, _ = sm.HealthCheck()
			}
		}()
	}

	wg.Wait()
	// If we get here without panic or deadlock, the test passes
}

// TestStoreManager_MultipleInstances tests that multiple StoreManagers can coexist.
func TestStoreManager_MultipleInstances(t *testing.T) {
	tempDir := t.TempDir()

	dir1 := filepath.Join(tempDir, "instance1")
	dir2 := filepath.Join(tempDir, "instance2")

	sm1, err := NewStoreManager(dir1)
	if err != nil {
		t.Fatalf("NewStoreManager() failed for instance1: %v", err)
	}
	defer sm1.Close()

	sm2, err := NewStoreManager(dir2)
	if err != nil {
		t.Fatalf("NewStoreManager() failed for instance2: %v", err)
	}
	defer sm2.Close()

	// Verify they have different base directories
	if sm1.BaseDir() == sm2.BaseDir() {
		t.Error("Multiple instances should have different base directories")
	}

	// Verify both have valid stores
	if sm1.Stats() == nil || sm2.Stats() == nil {
		t.Error("Both instances should have valid stores")
	}
}

// TestStoreManager_BusyTimeout tests custom busy timeout configuration.
func TestStoreManager_BusyTimeout(t *testing.T) {
	tempDir := t.TempDir()

	config := StoreManagerConfig{
		BaseDir:     tempDir,
		BusyTimeout: 100, // Very short timeout for testing
	}

	sm, err := NewStoreManagerWithConfig(config)
	if err != nil {
		t.Fatalf("NewStoreManagerWithConfig() failed: %v", err)
	}
	defer sm.Close()

	// Just verify it was created successfully
	if sm == nil {
		t.Fatal("StoreManager is nil")
	}
}
