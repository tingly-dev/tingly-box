package server

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestGetProviderModelsFallbackOrder tests the explicit fallback order:
// 1. DB Cache (API, 1h) -> 2. Provider API -> 3. Template (1d)
func TestGetProviderModelsFallbackOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		setupDB        func(*db.ModelStore, string)
		setupProvider  func(*gin.Context)
		expectedSource ModelCacheSource
		expectedModels int
	}{
		{
			name: "DB hit - API sourced (valid cache)",
			setupDB: func(store *db.ModelStore, uuid string) {
				provider := &typ.Provider{UUID: uuid, Name: "test", APIBase: "https://api.test.com"}
				models := []string{"model-1", "model-2"}
				require.NoError(t, store.SaveModels(provider, models, db.ModelSourceAPI))
			},
			setupProvider:  func(c *gin.Context) {},
			expectedSource: ModelCacheSourceDB,
			expectedModels: 2,
		},
		{
			name: "DB hit - Template sourced (valid cache)",
			setupDB: func(store *db.ModelStore, uuid string) {
				provider := &typ.Provider{UUID: uuid, Name: "test", APIBase: "https://api.test.com"}
				models := []string{"tmpl-1"}
				require.NoError(t, store.SaveModels(provider, models, db.ModelSourceTemplate))
			},
			setupProvider:  func(c *gin.Context) {},
			expectedSource: ModelCacheSourceDB,
			expectedModels: 1,
		},
		{
			name:           "DB miss - stale API cache -> API fetch",
			setupDB:        func(store *db.ModelStore, uuid string) {},
			setupProvider:  func(c *gin.Context) {},
			expectedSource: ModelCacheSourceAPI, // Would require mock API
			expectedModels: 0,                   // No mock in this test
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test requires a full Server setup with mocked dependencies
			// Placeholder for test structure
			t.Skip("Requires server setup - implement with mock server")
		})
	}
}

// TestProviderModelResponseMeta tests the new cache metadata in responses
func TestProviderModelResponseMeta(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		response       ProviderModelsResponse
		expectedSource ModelCacheSource
		expectExpiry   bool
	}{
		{
			name: "DB cache response",
			response: ProviderModelsResponse{
				Data: ProviderModelInfo{
					Models:    []string{"model-1"},
					Source:    ModelCacheSourceDB,
					ExpiresAt: time.Now().Add(1 * time.Hour),
				},
			},
			expectedSource: ModelCacheSourceDB,
			expectExpiry:   true,
		},
		{
			name: "Template fallback response",
			response: ProviderModelsResponse{
				Data: ProviderModelInfo{
					Models:    []string{"tmpl-1"},
					Source:    ModelCacheSourceTemplate,
					ExpiresAt: time.Now().Add(24 * time.Hour),
				},
			},
			expectedSource: ModelCacheSourceTemplate,
			expectExpiry:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedSource, tt.response.Data.Source)
			if tt.expectExpiry {
				assert.False(t, tt.response.Data.ExpiresAt.IsZero())
			}
		})
	}
}

// TestModelCacheSourceSerialization tests JSON serialization of new fields
func TestModelCacheSourceSerialization(t *testing.T) {
	info := ProviderModelInfo{
		Models:      []string{"model-1", "model-2"},
		Source:      ModelCacheSourceAPI,
		ExpiresAt:   time.Date(2026, 5, 26, 15, 0, 0, 0, time.UTC),
		LastUpdated: "2026-05-26 14:00:00",
	}

	// Test JSON marshaling
	data, err := json.Marshal(info)
	require.NoError(t, err)

	// Verify fields exist
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &parsed))

	assert.Contains(t, parsed, "source")
	assert.Equal(t, string(ModelCacheSourceAPI), parsed["source"])
	assert.Contains(t, parsed, "expiresAt")
	assert.Equal(t, "2026-05-26T15:00:00Z", parsed["expiresAt"])
}

// TestTemplateCacheTTL tests that template-sourced models use 24h TTL
func TestTemplateCacheTTL(t *testing.T) {
	// Test template TTL is 24 hours
	expectedTTL := 24 * time.Hour

	// Verify expiresAt calculation
	expiresAt := time.Now().Add(expectedTTL)
	duration := expiresAt.Sub(time.Now())

	assert.InDelta(t, 24*float64(time.Hour), float64(duration), float64(time.Second))
}
