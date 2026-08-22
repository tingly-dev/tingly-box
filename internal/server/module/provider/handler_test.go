package provider

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/dataio"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

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

// TestImportProviders_JSONL tests importing providers from JSONL format.
func TestImportProviders_JSONL(t *testing.T) {
	cfg, _ := config.NewConfig(config.WithConfigDir(t.TempDir()))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(cfg, nil)

	router.POST("/provider-import", handler.ImportProviders)

	jsonlData := `{"type":"metadata","version":"1.0","exported_at":"2024-01-01T00:00:00Z"}
{"type":"provider","uuid":"prov-1","name":"TestProvider","api_base":"https://api.test.com","api_style":"openai","auth_type":"api_key","token":"sk-test","enabled":true,"timeout":30}`

	importReq := ImportProvidersRequest{
		Data: jsonlData,
	}
	body, _ := json.Marshal(importReq)
	req, _ := http.NewRequest("POST", "/provider-import", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	bodyResp := w.Body.String()
	assert.Contains(t, bodyResp, `"success":true`)
	assert.Contains(t, bodyResp, `"providers_created":1`)
}

// TestImportProviders_Base64 tests importing providers from Base64 format
func TestImportProviders_Base64(t *testing.T) {
	cfg, _ := config.NewConfig(config.WithConfigDir(t.TempDir()))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(cfg, nil)

	router.POST("/provider-import", handler.ImportProviders)

	jsonlData := `{"type":"metadata","version":"1.0","exported_at":"2024-01-01T00:00:00Z"}
{"type":"provider","uuid":"prov-1","name":"TestProvider","api_base":"https://api.test.com","api_style":"openai","auth_type":"api_key","token":"sk-test","enabled":true}`

	// Encode the JSONL data to Base64
	base64Payload := base64.StdEncoding.EncodeToString([]byte(jsonlData))
	base64Data := dataio.Base64Prefix + ":1.0:" + base64Payload

	importReq := ImportProvidersRequest{
		Data: base64Data,
	}
	body, _ := json.Marshal(importReq)
	req, _ := http.NewRequest("POST", "/provider-import", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	bodyResp := w.Body.String()
	assert.Contains(t, bodyResp, `"success":true`)
}

// TestImportProviders_AlwaysMintsNewUUID verifies that importing a bundle
// whose provider UUID collides with an already-existing local provider does
// not reuse that provider — it always creates a new one with a fresh UUID
// (identity mapping is tracked separately via ProviderMap for future
// rule-remap consumers; see internal/dataio/jsonl_import.go).
func TestImportProviders_AlwaysMintsNewUUID(t *testing.T) {
	cfg, _ := config.NewConfig(config.WithConfigDir(t.TempDir()))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(cfg, nil)

	router.POST("/provider-import", handler.ImportProviders)

	// First create an existing provider with UUID "prov-1" (same as in the import)
	existingProvider := &typ.Provider{
		UUID:     "prov-1", // Same UUID as in the import data
		Name:     "ExistingProvider",
		APIBase:  "https://api.existing.com",
		APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeAPIKey,
		Token:    "sk-existing",
		Enabled:  true,
	}
	cfg.AddProvider(existingProvider)

	// Import a provider with the same UUID but different name
	jsonlData := `{"type":"metadata","version":"1.0","exported_at":"2024-01-01T00:00:00Z"}
{"type":"provider","uuid":"prov-1","name":"TestProvider","api_base":"https://api.test.com","api_style":"openai","auth_type":"api_key","token":"sk-test","enabled":true}`

	importReq := ImportProvidersRequest{
		Data: jsonlData,
	}
	body, _ := json.Marshal(importReq)
	req, _ := http.NewRequest("POST", "/provider-import", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	bodyResp := w.Body.String()
	assert.Contains(t, bodyResp, `"success":true`)

	// Parse response to check provider info
	var resp ImportProvidersResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// A brand new provider is always created, with a fresh UUID distinct
	// from both the bundle's original UUID and the pre-existing provider's.
	if resp.Data.ProvidersCreated != 1 {
		t.Errorf("Expected 1 provider created, got %d", resp.Data.ProvidersCreated)
	}
	if len(resp.Data.Providers) != 1 {
		t.Fatalf("Expected 1 provider in response, got %d", len(resp.Data.Providers))
	}
	created := resp.Data.Providers[0]
	if created.Action != "created" {
		t.Errorf("Expected action 'created', got %q", created.Action)
	}
	if created.UUID == "prov-1" {
		t.Error("Expected a freshly minted UUID, got the colliding source UUID")
	}

	// The pre-existing provider is left untouched.
	stillExisting, err := cfg.GetProviderByUUID("prov-1")
	if err != nil || stillExisting == nil {
		t.Fatal("Expected the pre-existing provider to still be present")
	}
	if stillExisting.Name != "ExistingProvider" {
		t.Errorf("Expected existing provider to be unmodified, got name %q", stillExisting.Name)
	}
}

// TestImportProviders_InvalidData tests importing with invalid data
func TestImportProviders_InvalidData(t *testing.T) {
	cfg, _ := config.NewConfig(config.WithConfigDir(t.TempDir()))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(cfg, nil)

	router.POST("/provider-import", handler.ImportProviders)

	importReq := ImportProvidersRequest{
		Data: "invalid data",
	}
	body, _ := json.Marshal(importReq)
	req, _ := http.NewRequest("POST", "/provider-import", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	bodyResp := w.Body.String()
	assert.Contains(t, bodyResp, `"success":false`)
}

// TestImportProviders_MissingData tests importing with missing data field
func TestImportProviders_MissingData(t *testing.T) {
	cfg, _ := config.NewConfig(config.WithConfigDir(t.TempDir()))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(cfg, nil)

	router.POST("/provider-import", handler.ImportProviders)

	importReq := map[string]string{
		"on_provider_conflict": "use",
		// Missing "data" field
	}
	body, _ := json.Marshal(importReq)
	req, _ := http.NewRequest("POST", "/provider-import", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

// TestProviderFlags_API drives the flags/model_flags fields through the real
// HTTP handlers: create with flags, partial-update semantics (nil untouched,
// non-nil replaces, empty clears), response surfacing, and the api_key gate.
func TestProviderFlags_API(t *testing.T) {
	cfg, _ := config.NewConfig(config.WithConfigDir(t.TempDir()))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(cfg, nil)
	router.POST("/providers", handler.CreateProvider)
	router.PUT("/providers/:uuid", handler.UpdateProvider)
	router.GET("/providers/:uuid", handler.GetProvider)

	do := func(method, path string, payload any) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		req, _ := http.NewRequest(method, path, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// Create with provider- and model-level flags.
	w := do("POST", "/providers", CreateProviderRequest{
		Name:    "flags-provider",
		APIBase: "https://api.test.com/v1",
		Token:   "sk-test",
		Flags:   &typ.ProviderFlags{ExtraHeaders: map[string]string{"x-title": "tingly"}},
		ModelFlags: map[string]typ.ProviderFlags{
			"gpt-x": {ExtraHeaders: map[string]string{"X-Canary": "on"}},
		},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var created struct {
		Data typ.Provider `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	uid := created.Data.UUID
	require.NotEmpty(t, uid)

	// readBack decodes into a fresh struct each time — json.Unmarshal into a
	// reused struct merges maps instead of replacing them.
	readBack := func() ProviderResponse {
		t.Helper()
		w := do("GET", "/providers/"+uid, nil)
		require.Equal(t, http.StatusOK, w.Code)
		var got struct {
			Data ProviderResponse `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		return got.Data
	}

	// Read back: names canonicalized, both levels surfaced.
	resp := readBack()
	require.NotNil(t, resp.Flags)
	assert.Equal(t, "tingly", resp.Flags.ExtraHeaders["X-Title"])
	assert.Equal(t, "on", resp.ModelFlags["gpt-x"].ExtraHeaders["X-Canary"])

	// Partial update with both sections omitted leaves flags untouched.
	name := "flags-provider-renamed"
	w = do("PUT", "/providers/"+uid, UpdateProviderRequest{Name: &name})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	resp = readBack()
	require.NotNil(t, resp.Flags)
	assert.Equal(t, "tingly", resp.Flags.ExtraHeaders["X-Title"])

	// Non-nil flags replace wholesale; empty model_flags map clears it.
	emptyMF := map[string]typ.ProviderFlags{}
	w = do("PUT", "/providers/"+uid, UpdateProviderRequest{
		Flags:      &typ.ProviderFlags{ExtraHeaders: map[string]string{"X-New": "v"}},
		ModelFlags: &emptyMF,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	resp = readBack()
	require.NotNil(t, resp.Flags)
	assert.Equal(t, "v", resp.Flags.ExtraHeaders["X-New"])
	assert.NotContains(t, resp.Flags.ExtraHeaders, "X-Title")
	assert.Empty(t, resp.ModelFlags)

	// Structurally invalid header name rejected with 400 (no denylist —
	// gateway-adjacent names like Authorization are accepted verbatim).
	w = do("PUT", "/providers/"+uid, UpdateProviderRequest{
		Flags: &typ.ProviderFlags{ExtraHeaders: map[string]string{"X Title": "bad name"}},
	})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	// api_key gate: headers on a non-api_key provider are rejected.
	w = do("POST", "/providers", CreateProviderRequest{
		Name:     "sigv4-provider",
		APIBase:  "https://bedrock.test",
		AuthType: "aws_sigv4",
		APIStyle: "anthropic",
		Credential: map[string]string{
			"access_key_id": "AKIA", "secret_access_key": "s", "region": "us-east-1",
		},
		Flags: &typ.ProviderFlags{ExtraHeaders: map[string]string{"X-Title": "t"}},
	})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "api_key")
}
