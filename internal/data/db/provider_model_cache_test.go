package db

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// setupTestStore creates a temporary model store for testing
func setupTestStore(t *testing.T) *ModelStore {
	tmpDir := t.TempDir()
	// Create the required subdirectory structure
	dbDir := filepath.Join(tmpDir, "db")
	require.NoError(t, os.MkdirAll(dbDir, 0700))

	store, err := NewModelStore(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, store)
	return store
}

// TestGetModelsTemplateTTL tests that template-sourced models respect TTL
func TestGetModelsTemplateTTL(t *testing.T) {
	store := setupTestStore(t)

	provider := &typ.Provider{
		UUID:    "test-uuid",
		Name:    "Test Provider",
		APIBase: "https://api.test.com",
	}
	models := []string{"template-model-1", "template-model-2"}

	// Save template models
	err := store.SaveModels(provider, models, ModelSourceTemplate)
	require.NoError(t, err)

	// Fresh template cache should return models
	result := store.GetModels(provider.UUID, 24*time.Hour)
	assert.Equal(t, models, result)

	// Note: Can't test expiration without manual timestamp manipulation
	// The TTL check uses time.Since(record.LastUpdated)
}

// TestGetModelsAPITTL tests that API-sourced models respect TTL
func TestGetModelsAPITTL(t *testing.T) {
	store := setupTestStore(t)

	provider := &typ.Provider{
		UUID:    "test-uuid",
		Name:    "Test Provider",
		APIBase: "https://api.test.com",
	}
	models := []string{"api-model-1"}

	// Save API models
	err := store.SaveModels(provider, models, ModelSourceAPI)
	require.NoError(t, err)

	// Fresh API cache should return models
	result := store.GetModels(provider.UUID, 1*time.Hour)
	assert.Equal(t, models, result)

	// Expired API cache should return empty
	result = store.GetModels(provider.UUID, 1*time.Nanosecond)
	assert.Empty(t, result)
}

// TestGetModelsNoCache tests that no stored models returns empty
func TestGetModelsNoCache(t *testing.T) {
	store := setupTestStore(t)

	result := store.GetModels("non-existent", 1*time.Hour)
	assert.Empty(t, result)
}

// TestSaveModelsOverwrite tests that saving new models overwrites existing
func TestSaveModelsOverwrite(t *testing.T) {
	store := setupTestStore(t)

	provider := &typ.Provider{
		UUID:    "test-uuid",
		Name:    "Test Provider",
		APIBase: "https://api.test.com",
	}

	// Save initial models
	models1 := []string{"model-1"}
	err := store.SaveModels(provider, models1, ModelSourceAPI)
	require.NoError(t, err)

	// Overwrite with new models
	models2 := []string{"model-2", "model-3"}
	err = store.SaveModels(provider, models2, ModelSourceTemplate)
	require.NoError(t, err)

	// Should get new models
	result := store.GetModels(provider.UUID, 1*time.Hour)
	assert.Equal(t, models2, result)

	// Source should be updated
	record := store.GetAllModelRecords()
	require.Len(t, record, 1)
	assert.Equal(t, ModelSourceTemplate, record[0].Source)
}

// TestRemoveProviderModels tests removal of provider models
func TestRemoveProviderModels(t *testing.T) {
	store := setupTestStore(t)

	provider := &typ.Provider{
		UUID:    "test-uuid",
		Name:    "Test Provider",
		APIBase: "https://api.test.com",
	}
	models := []string{"model-1"}

	// Save models
	err := store.SaveModels(provider, models, ModelSourceAPI)
	require.NoError(t, err)

	// Verify exists
	result := store.GetModels(provider.UUID, 1*time.Hour)
	assert.NotEmpty(t, result)

	// Remove
	err = store.RemoveProvider(provider.UUID)
	require.NoError(t, err)

	// Verify gone
	result = store.GetModels(provider.UUID, 1*time.Hour)
	assert.Empty(t, result)
}
