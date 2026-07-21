package data

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestNewTemplateManager tests the TemplateManager constructor
func TestNewTemplateManager(t *testing.T) {
	tests := []struct {
		name      string
		githubURL string
	}{
		{
			name:      "With GitHub URL",
			githubURL: "https://example.com/templates.json",
		},
		{
			name:      "Without GitHub URL",
			githubURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewTemplateManager(tt.githubURL)
			if tm == nil {
				t.Fatal("NewTemplateManager returned nil")
			}
			if tm.githubURL != tt.githubURL {
				t.Errorf("expected githubURL %q, got %q", tt.githubURL, tm.githubURL)
			}
			if tm.httpClient == nil {
				t.Error("httpClient should be initialized")
			}
			if tm.templates == nil {
				t.Error("templates map should be initialized")
			}
		})
	}
}

// TestTemplateManagerInitialize tests initialization with embedded templates
func TestTemplateManagerInitialize(t *testing.T) {
	tm := NewTemplateManager("")
	if err := tm.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Verify templates were loaded
	allTemplates := tm.GetAllTemplates()
	if len(allTemplates) == 0 {
		t.Error("Expected templates to be loaded, got empty map")
	}

	// Verify version was set
	version := tm.GetVersion()
	if version == "" {
		t.Error("Expected version to be set")
	}
}

// TestTemplateManagerGetTemplate tests retrieving individual templates
func TestTemplateManagerGetTemplate(t *testing.T) {
	tm := NewTemplateManager("")
	if err := tm.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	tests := []struct {
		name        string
		templateID  string
		expectError bool
		verifyFunc  func(*testing.T, *ProviderTemplate)
	}{
		{
			name:        "Get existing template - openai-com",
			templateID:  "openai-com",
			expectError: false,
			verifyFunc: func(t *testing.T, tmpl *ProviderTemplate) {
				if tmpl.ID != "openai-com" {
					t.Errorf("expected ID 'openai-com', got %q", tmpl.ID)
				}
				if tmpl.Name != "OpenAI" {
					t.Errorf("expected Name 'OpenAI', got %q", tmpl.Name)
				}
				if tmpl.CanonicalDomain != "api.openai.com" {
					t.Errorf("expected CanonicalDomain 'api.openai.com', got %q", tmpl.CanonicalDomain)
				}
				if tmpl.VendorFamily != "openai" {
					t.Errorf("expected VendorFamily 'openai', got %q", tmpl.VendorFamily)
				}
				if tmpl.BaseURLOpenAI != "https://api.openai.com/v1" {
					t.Errorf("expected BaseURLOpenAI 'https://api.openai.com/v1', got %q", tmpl.BaseURLOpenAI)
				}
				if !tmpl.SupportsModelsEndpoint {
					t.Error("expected SupportsModelsEndpoint to be true for openai-com")
				}
				// Verify Models array structure
				if len(tmpl.Models) == 0 {
					t.Error("expected openai-com to have predefined models")
				}
				// Check first model has correct structure
				for _, m := range tmpl.Models {
					if m.ID == "" {
						t.Error("expected model ID to be set")
					}
					if m.Context > 0 {
						// Model with context window defined
						break
					}
				}
			},
		},
		{
			name:        "Get existing template - minimaxi-com",
			templateID:  "minimaxi-com",
			expectError: false,
			verifyFunc: func(t *testing.T, tmpl *ProviderTemplate) {
				if tmpl.ID != "minimaxi-com" {
					t.Errorf("expected ID 'minimaxi-com', got %q", tmpl.ID)
				}
				if len(tmpl.Models) == 0 {
					t.Error("expected minimaxi-com to have predefined models")
				}
				if tmpl.SupportsModelsEndpoint {
					t.Error("expected SupportsModelsEndpoint to be false for minimaxi-com")
				}
				// Verify Models are ModelInfo structs
				for _, m := range tmpl.Models {
					if m.ID == "" {
						t.Error("expected model ID to be set")
					}
					// Context should be populated for Minimax models
					if m.Context == 0 {
						t.Errorf("expected context window for model %s", m.ID)
					}
				}
			},
		},
		{
			name:        "Get non-existent template",
			templateID:  "nonexistent-provider",
			expectError: true,
			verifyFunc:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := tm.GetTemplate(tt.templateID)
			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if tmpl == nil {
					t.Fatal("expected template, got nil")
				}
				if tt.verifyFunc != nil {
					tt.verifyFunc(t, tmpl)
				}
			}
		})
	}
}

// TestTemplateManagerFetchTemplates tests template fetching from various sources
func TestTemplateManagerFetchTemplates(t *testing.T) {
	tests := []struct {
		name        string
		githubURL   string
		expectError bool
	}{
		{
			name:        "Successful fetch from GitHub",
			githubURL:   "https://raw.githubusercontent.com/tingly-dev/tingly-box/main/internal/data/providers.json",
			expectError: false, // File exists on main branch; expectError=false means success is expected
		},
		{
			name:        "No GitHub URL configured",
			githubURL:   "",
			expectError: true,
		},
		{
			name:        "Invalid GitHub URL",
			githubURL:   "https://raw.githubusercontent.com/tingly-dev/tingly-box/main/nonexistent.json",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewTemplateManager(tt.githubURL)
			// Initialize first to load embedded templates
			_ = tm.Initialize(context.Background())

			registry, err := tm.FetchTemplates(context.Background())
			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if registry == nil {
					t.Error("expected registry, got nil")
				}
				if len(registry.Providers) == 0 {
					t.Error("expected providers in registry")
				}
			}
		})
	}
}

// TestTemplateManagerGetModelsForProvider tests the GetModelsForProvider method
func TestTemplateManagerGetModelsForProvider(t *testing.T) {
	tests := []struct {
		name           string
		githubURL      string
		provider       *typ.Provider
		expectError    bool
		expectModels   bool
		expectedSource TemplateSource
	}{
		{
			name:      "Provider with predefined models from embedded - minimaxi-com",
			githubURL: "",
			provider: &typ.Provider{
				Name:     "my-minimax",
				APIBase:  "https://api.minimaxi.com/v1",
				APIStyle: protocol.APIStyleOpenAI,
			},
			expectError:    false,
			expectModels:   true,
			expectedSource: TemplateSourceLocal,
		},
		{
			name:      "Provider with predefined models from embedded - openai-com",
			githubURL: "",
			provider: &typ.Provider{
				Name:     "my-openai",
				APIBase:  "https://api.openai.com/v1",
				APIStyle: protocol.APIStyleOpenAI,
			},
			expectError:    false,
			expectModels:   true,
			expectedSource: TemplateSourceLocal,
		},
		{
			name:      "Non-existent provider",
			githubURL: "",
			provider: &typ.Provider{
				Name:    "nonexistent",
				APIBase: "https://nonexistent.example.com/v1",
			},
			expectError:    true,
			expectModels:   false,
			expectedSource: TemplateSourceLocal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewTemplateManager(tt.githubURL)
			if err := tm.Initialize(context.Background()); err != nil {
				t.Fatalf("Initialize failed: %v", err)
			}

			models, source, err := tm.GetModelsForProvider(tt.provider)

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil && tt.expectModels {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			if tt.expectModels && len(models) == 0 {
				t.Error("expected models, got empty slice")
			}

			if source != tt.expectedSource {
				t.Errorf("expected source %v, got %v", tt.expectedSource, source)
			}
		})
	}
}

// TestValidateTemplate tests template validation
func TestValidateTemplate(t *testing.T) {
	tests := []struct {
		name        string
		template    *ProviderTemplate
		expectError bool
	}{
		{
			name: "Valid template",
			template: &ProviderTemplate{
				ID:            "test",
				Name:          "Test Provider",
				BaseURLOpenAI: "https://api.test.com/v1",
			},
			expectError: false,
		},
		{
			name: "Missing ID",
			template: &ProviderTemplate{
				Name:          "Test Provider",
				BaseURLOpenAI: "https://api.test.com/v1",
			},
			expectError: true,
		},
		{
			name: "Missing Name",
			template: &ProviderTemplate{
				ID:            "test",
				BaseURLOpenAI: "https://api.test.com/v1",
			},
			expectError: true,
		},
		{
			name: "Missing base_url for non-OAuth template",
			template: &ProviderTemplate{
				ID:   "test",
				Name: "Test Provider",
			},
			expectError: true,
		},
		{
			name: "Valid with only Anthropic URL",
			template: &ProviderTemplate{
				ID:               "test",
				Name:             "Test Provider",
				BaseURLAnthropic: "https://api.test.com",
			},
			expectError: false,
		},
		{
			name: "Valid OAuth template with auth_type and oauth_provider",
			template: &ProviderTemplate{
				ID:            "test_oauth",
				Name:          "Test OAuth Provider",
				AuthType:      "oauth",
				OAuthProvider: "claude_code",
			},
			expectError: false,
		},
		{
			name: "OAuth template without oauth_provider field",
			template: &ProviderTemplate{
				ID:       "test_oauth",
				Name:     "Test OAuth Provider",
				AuthType: "oauth",
			},
			expectError: true,
		},
		{
			name: "OAuth template without base_url is valid",
			template: &ProviderTemplate{
				ID:            "test_oauth",
				Name:          "Test OAuth Provider",
				AuthType:      "oauth",
				OAuthProvider: "claude_code",
			},
			expectError: false,
		},
		{
			name: "OAuth template with both oauth_provider and base_url is also valid",
			template: &ProviderTemplate{
				ID:            "test_oauth",
				Name:          "Test OAuth Provider",
				AuthType:      "oauth",
				OAuthProvider: "claude_code",
				BaseURLOpenAI: "https://api.test.com/v1",
			},
			expectError: false,
		},
		{
			name: "Template with auth_type=key and oauth_provider is unusual but not invalid",
			template: &ProviderTemplate{
				ID:            "test",
				Name:          "Test Provider",
				AuthType:      "key",
				OAuthProvider: "some_provider",
				BaseURLOpenAI: "https://api.test.com/v1",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTemplate(tt.template)
			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestTemplateManagerConcurrentAccess tests concurrent access to templates
func TestTemplateManagerConcurrentAccess(t *testing.T) {
	tm := NewTemplateManager("")
	if err := tm.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	done := make(chan bool)

	// Concurrent readers
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()
			for j := 0; j < 100; j++ {
				_, _ = tm.GetTemplate("openai")
				_ = tm.GetVersion()
				_ = tm.GetAllTemplates()
			}
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify data integrity
	templates := tm.GetAllTemplates()
	if len(templates) == 0 {
		t.Error("Templates map should not be empty after concurrent access")
	}
}

// TestEmbeddedTemplatesAreValid exhaustively loads every provider template from the
// embedded providers.json and checks it individually, instead of spot-checking a
// couple of hand-picked provider ids. This is meant to catch structural mistakes
// (missing required fields, malformed model entries) introduced by hand-edits to
// providers.json before they reach production.
func TestEmbeddedTemplatesAreValid(t *testing.T) {
	tm := NewEmbeddedOnlyTemplateManager()
	if err := tm.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	templates := tm.GetAllTemplates()
	if len(templates) < 50 {
		t.Fatalf("expected at least 50 embedded provider templates, got %d", len(templates))
	}

	for id, tmpl := range templates {
		t.Run(id, func(t *testing.T) {
			if tmpl.ID != id {
				t.Errorf("map key %q does not match template.ID %q", id, tmpl.ID)
			}
			if err := ValidateTemplate(tmpl); err != nil {
				t.Errorf("template %q failed ValidateTemplate: %v", id, err)
			}
			if tmpl.VendorFamily == "" {
				t.Errorf("template %q missing vendor_family", id)
			}
			if tmpl.Region == "" {
				t.Errorf("template %q missing region", id)
			}
			if tmpl.Plan == "" {
				t.Errorf("template %q missing plan", id)
			}

			seenModelID := make(map[string]bool, len(tmpl.Models))
			for _, m := range tmpl.Models {
				if m.ID == "" {
					t.Errorf("template %q has a model with an empty id", id)
					continue
				}
				if seenModelID[m.ID] {
					t.Errorf("template %q has duplicate model id %q", id, m.ID)
				}
				seenModelID[m.ID] = true
				if m.Context < 0 {
					t.Errorf("template %q model %q has negative context %d", id, m.ID, m.Context)
				}
				if m.MaxOutput < 0 {
					t.Errorf("template %q model %q has negative max_output %d", id, m.ID, m.MaxOutput)
				}
			}
		})
	}
}

// TestEmbeddedTemplatesLastUpdatedSourcesPaired checks the optional per-provider
// last_updated/sources convention documented in providers.json's
// _naming_rules.provider_meta_fields: whenever one is set, the other must be too,
// and sources must be a non-empty list. These fields aren't part of the ProviderTemplate
// Go struct (unknown JSON fields are silently ignored by encoding/json), so this reads
// the embedded bytes directly rather than through the typed loader.
func TestEmbeddedTemplatesLastUpdatedSourcesPaired(t *testing.T) {
	var raw struct {
		Providers map[string]map[string]interface{} `json:"providers"`
	}
	if err := json.Unmarshal(embeddedTemplatesJSON, &raw); err != nil {
		t.Fatalf("failed to parse embedded templates as raw JSON: %v", err)
	}
	if len(raw.Providers) < 50 {
		t.Fatalf("expected at least 50 providers in raw JSON, got %d", len(raw.Providers))
	}

	for id, fields := range raw.Providers {
		_, hasLastUpdated := fields["last_updated"]
		sources, hasSources := fields["sources"]
		if hasLastUpdated != hasSources {
			t.Errorf("provider %q: last_updated and sources must be set together (has last_updated=%v, has sources=%v)", id, hasLastUpdated, hasSources)
			continue
		}
		if !hasSources {
			continue
		}
		list, ok := sources.([]interface{})
		if !ok || len(list) == 0 {
			t.Errorf("provider %q: sources must be a non-empty array", id)
			continue
		}
		for i, s := range list {
			url, ok := s.(string)
			if !ok || url == "" {
				t.Errorf("provider %q: sources[%d] must be a non-empty string", id, i)
			}
		}
	}
}

// TestTemplateManagerHTTPTimeout tests HTTP client timeout
func TestTemplateManagerHTTPTimeout(t *testing.T) {
	// Create a server that delays response
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer svr.Close()

	tm := NewTemplateManager(svr.URL)
	if tm.httpClient == nil {
		t.Fatal("httpClient should be initialized")
	}

	// Verify timeout is set
	if tm.httpClient.Timeout <= 0 {
		t.Error("Expected positive timeout, got", tm.httpClient.Timeout)
	}
}
