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

// TestSaveModelsWithRaw pins that a successful real upstream fetch persists
// both the model list and the raw payload, and clears any prior error.
func TestSaveModelsWithRaw(t *testing.T) {
	store := setupTestStore(t)

	provider := &typ.Provider{
		UUID:    "raw-uuid",
		Name:    "Raw Provider",
		APIBase: "https://api.test.com",
	}

	// Seed a prior failure so we can verify SaveModelsWithRaw clears it.
	require.NoError(t, store.SaveFetchFailure(provider, "boom", nil, time.Time{}))

	models := []string{"m-1", "m-2"}
	raw := []byte(`{"data":[{"id":"m-1"},{"id":"m-2"}]}`)
	require.NoError(t, store.SaveModelsWithRaw(provider, models, ModelSourceAPI, raw))

	// Models + raw persisted.
	assert.Equal(t, models, store.GetModels(provider.UUID, time.Hour))
	assert.Equal(t, string(raw), store.GetRawResponse(provider.UUID))

	// Prior error cleared.
	records := store.GetAllModelRecords()
	require.Len(t, records, 1)
	assert.Nil(t, records[0].LastError)
	assert.Nil(t, records[0].LastErrorAt)
}

// TestSaveFetchFailure_PreservesModels pins that recording a fetch error does
// not clobber a pre-existing model list — a stale list is more useful than
// empty — while still recording the error and any partial raw body.
func TestSaveFetchFailure_PreservesModels(t *testing.T) {
	store := setupTestStore(t)

	provider := &typ.Provider{
		UUID:    "fail-uuid",
		Name:    "Fail Provider",
		APIBase: "https://api.test.com",
	}
	models := []string{"cached-1"}
	require.NoError(t, store.SaveModels(provider, models, ModelSourceAPI))

	raw := []byte(`{"error":"404"}`)
	require.NoError(t, store.SaveFetchFailure(provider, "upstream returned 404", raw, time.Time{}))

	// Existing models must survive.
	assert.Equal(t, models, store.GetModels(provider.UUID, time.Hour))

	records := store.GetAllModelRecords()
	require.Len(t, records, 1)
	require.NotNil(t, records[0].LastError)
	assert.Equal(t, "upstream returned 404", *records[0].LastError)
	require.NotNil(t, records[0].LastErrorAt)
	assert.Equal(t, string(raw), *records[0].RawResponse)
}

// TestSaveFetchFailure_EmptyErrorIsNoop pins that an empty error string is a
// no-op (the public guard in SaveFetchFailure).
func TestSaveFetchFailure_EmptyErrorIsNoop(t *testing.T) {
	store := setupTestStore(t)
	provider := &typ.Provider{UUID: "noop-uuid", Name: "P", APIBase: "x"}
	require.NoError(t, store.SaveFetchFailure(provider, "", []byte(`{}`), time.Time{}))
	assert.Empty(t, store.GetAllModelRecords())
}

func TestSaveFetchFailure_DoesNotCreateModelCache(t *testing.T) {
	store := setupTestStore(t)
	provider := &typ.Provider{UUID: "failure-only", Name: "P", APIBase: "x"}
	require.NoError(t, store.SaveFetchFailure(provider, "upstream failed", nil, time.Time{}))

	assert.False(t, store.HasModels(provider.UUID))
	assert.Empty(t, store.GetAllProviders())
	_, updated, exists := store.GetProviderInfo(provider.UUID)
	assert.False(t, exists)
	assert.Empty(t, updated)
	_, failureExists := store.GetFetchFailure(provider.UUID)
	assert.True(t, failureExists)
}
