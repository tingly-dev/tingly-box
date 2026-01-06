package template

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tingly-box/internal/config/typ"
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
	if err := tm.Initialize(); err != nil {
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
	if err := tm.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	tests := []struct {
		name        string
		templateID  string
		expectError bool
		verifyFunc  func(*testing.T, *ProviderTemplate)
	}{
		{
			name:        "Get existing template - openai",
			templateID:  "openai",
			expectError: false,
			verifyFunc: func(t *testing.T, tmpl *ProviderTemplate) {
				if tmpl.ID != "openai" {
					t.Errorf("expected ID 'openai', got %q", tmpl.ID)
				}
				if tmpl.Name != "OpenAI" {
					t.Errorf("expected Name 'OpenAI', got %q", tmpl.Name)
				}
				if tmpl.BaseURLOpenAI != "https://api.openai.com/v1" {
					t.Errorf("expected BaseURLOpenAI 'https://api.openai.com/v1', got %q", tmpl.BaseURLOpenAI)
				}
				if !tmpl.SupportsModelsEndpoint {
					t.Error("expected SupportsModelsEndpoint to be true for openai")
				}
			},
		},
		{
			name:        "Get existing template - minimax",
			templateID:  "minimax",
			expectError: false,
			verifyFunc: func(t *testing.T, tmpl *ProviderTemplate) {
				if tmpl.ID != "minimax" {
					t.Errorf("expected ID 'minimax', got %q", tmpl.ID)
				}
				if len(tmpl.Models) == 0 {
					t.Error("expected minimax to have predefined models")
				}
				if tmpl.SupportsModelsEndpoint {
					t.Error("expected SupportsModelsEndpoint to be false for minimax")
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

// TestTemplateManagerFetchFromGitHub tests GitHub template fetching
func TestTemplateManagerFetchFromGitHub(t *testing.T) {
	tests := []struct {
		name        string
		githubURL   string
		expectError bool
	}{
		{
			name:        "Successful fetch from GitHub",
			githubURL:   "https://raw.githubusercontent.com/tingly-dev/tingly-box/main/internal/config/provider_templates.json",
			expectError: false,
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
			_ = tm.Initialize()

			registry, err := tm.FetchFromGitHub()
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
		providerName   string
		expectError    bool
		expectModels   bool
		expectedSource TemplateSource
	}{
		{
			name:           "Provider with predefined models from embedded - minimax",
			githubURL:      "",
			providerName:   "minimax",
			expectError:    false,
			expectModels:   true,
			expectedSource: TemplateSourceLocal,
		},
		{
			name:           "Provider with predefined models from GitHub - minimax",
			githubURL:      "https://raw.githubusercontent.com/tingly-dev/tingly-box/main/internal/config/provider_templates/provider_templates.json",
			providerName:   "minimax",
			expectError:    false,
			expectModels:   true,
			expectedSource: TemplateSourceGitHub,
		},
		{
			name:           "Provider with empty models but supports endpoint - openai",
			githubURL:      "",
			providerName:   "openai",
			expectError:    false,
			expectModels:   false, // Empty models list, but no error
			expectedSource: TemplateSourceLocal,
		},
		{
			name:           "Non-existent provider",
			githubURL:      "",
			providerName:   "nonexistent",
			expectError:    true,
			expectModels:   false,
			expectedSource: TemplateSourceLocal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewTemplateManager(tt.githubURL)
			if err := tm.Initialize(); err != nil {
				t.Fatalf("Initialize failed: %v", err)
			}

			provider := &typ.Provider{Name: tt.providerName}
			models, source, err := tm.GetModelsForProvider(provider)

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
			name: "Missing both base URLs",
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
	if err := tm.Initialize(); err != nil {
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
