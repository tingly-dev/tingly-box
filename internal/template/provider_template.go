package template

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

//go:embed provider_templates.json
var embeddedTemplatesJSON []byte

const DefaultTemplateHTTPTimeout = 30 * time.Second // Default HTTP timeout for fetching templates

const DefaultTemplateCacheTTL = 12 * time.Hour // Default TTL for template cache

const TemplateCacheFileName = "provider_template.json"

const TemplateGitHubURL = "https://raw.githubusercontent.com/tingly-dev/tingly-box/main/internal/template/provider_templates.json"

// CapabilitySchema defines the structure for capability schemas like web_search
type CapabilitySchema struct {
	BuiltIn      bool         `json:"built_in"`
	ToolType     string       `json:"tool_type,omitempty"`
	ToolName     string       `json:"tool_name,omitempty"`
	DocURL       string       `json:"doc_url,omitempty"`
	ResultFormat ResultFormat `json:"result_format,omitempty"`
}

// ResultFormat describes how to format tool results
type ResultFormat struct {
	Type        string                 `json:"type"`
	Description string                 `json:"description,omitempty"`
	Structure   map[string]interface{} `json:"structure,omitempty"`
}

// ProviderTemplate represents a predefined provider configuration template
type ProviderTemplate struct {
	ID                     string            `json:"id"`
	Name                   string            `json:"name"`
	Status                 string            `json:"status"` // "active", "deprecated", etc.
	Valid                  bool              `json:"valid"`
	Website                string            `json:"website"`
	Description            string            `json:"description"`
	Type                   string            `json:"type"` // "official", "reseller", etc.
	APIDoc                 string            `json:"api_doc"`
	ModelDoc               string            `json:"model_doc"`
	PricingDoc             string            `json:"pricing_doc"`
	BaseURLOpenAI          string            `json:"base_url_openai,omitempty"`
	BaseURLAnthropic       string            `json:"base_url_anthropic,omitempty"`
	Models                 []string          `json:"models"`                 // List of model IDs
	ModelLimits            map[string]int    `json:"model_limits,omitempty"` // Model name -> max_tokens mapping
	SupportsModelsEndpoint bool              `json:"supports_models_endpoint"`
	Tags                   []string          `json:"tags,omitempty"`
	Metadata               map[string]string `json:"metadata,omitempty"`
	OAuthProvider          string            `json:"oauth_provider,omitempty"`    // OAuth provider type for oauth type providers
	AuthType               string            `json:"auth_type,omitempty"`         // "oauth", "key"
	WebSearchSchema        string            `json:"web_search_schema,omitempty"` // Reference to capability schema for web_search
}

// ProviderTemplateRegistry represents the provider template registry structure from GitHub
type ProviderTemplateRegistry struct {
	Providers         map[string]*ProviderTemplate `json:"providers"`
	CapabilitySchemas map[string]*CapabilitySchema `json:"capability_schemas,omitempty"`
	Version           string                       `json:"version"`
	LastUpdated       string                       `json:"last_updated"`
}

// TemplateSource tracks where templates were loaded from
type TemplateSource int

const (
	// TemplateSourceGitHub - From GitHub templates
	TemplateSourceGitHub TemplateSource = iota
	// TemplateSourceLocal - From local embedded templates
	TemplateSourceLocal
)

// TemplateSourcePreference defines the priority order for loading templates
type TemplateSourcePreference int

const (
	// PreferenceDefault: Cache -> GitHub -> Embedded
	PreferenceDefault TemplateSourcePreference = iota
	// PreferenceEmbedded: Embedded only (no network requests)
	PreferenceEmbedded
	// PreferenceEmbeddedFirst: Embedded -> Cache -> GitHub
	PreferenceEmbeddedFirst
)

// TemplateManager manages provider templates with -tier fallback
type TemplateManager struct {
	templates         map[string]*ProviderTemplate // Current templates from GitHub or embedded
	embedded          map[string]*ProviderTemplate // Embedded templates (immutable fallback)
	capabilitySchemas map[string]*CapabilitySchema // Current capability schemas
	embeddedSchemas   map[string]*CapabilitySchema // Embedded capability schemas
	mu                sync.RWMutex
	lastUpdated       time.Time      // Last update timestamp
	version           string         // Template version
	source            TemplateSource // Current source: GitHub or Local
	sourceMu          sync.RWMutex
	etag              string // For conditional GitHub requests
	etagMu            sync.RWMutex
	githubURL         string                   // Empty means no GitHub sync, only embedded templates
	sourcePreference  TemplateSourcePreference // Priority order for loading templates
	httpClient        *http.Client
	cachePath         string        // Path to cache file
	cacheTTL          time.Duration // Cache TTL (default 24h)
}

func NewDefaultTemplateManager() *TemplateManager {
	return NewTemplateManagerWithPreference(TemplateGitHubURL, PreferenceDefault)
}

// NewEmbeddedOnlyTemplateManager creates a template manager that only uses embedded templates
// This is useful for development, testing, or offline scenarios
func NewEmbeddedOnlyTemplateManager() *TemplateManager {
	return NewTemplateManagerWithPreference("", PreferenceEmbedded)
}

// NewTemplateManager creates a new template manager with default preference.
// If githubURL is empty, only embedded templates will be used (no GitHub sync).
func NewTemplateManager(githubURL string) *TemplateManager {
	return NewTemplateManagerWithPreference(githubURL, PreferenceDefault)
}

// NewTemplateManagerWithPreference creates a new template manager with specified source preference.
// If githubURL is empty, only embedded templates will be used (no GitHub sync).
func NewTemplateManagerWithPreference(githubURL string, preference TemplateSourcePreference) *TemplateManager {
	configDir := constant.GetTinglyConfDir()
	return &TemplateManager{
		githubURL:         githubURL,
		sourcePreference:  preference,
		templates:         make(map[string]*ProviderTemplate),
		capabilitySchemas: make(map[string]*CapabilitySchema),
		httpClient: &http.Client{
			Timeout: DefaultTemplateHTTPTimeout,
		},
		cachePath: configDir, // Will store in .tingly-box directory
		cacheTTL:  DefaultTemplateCacheTTL,
	}
}

// GetTemplate returns a provider template by ID
func (tm *TemplateManager) GetTemplate(id string) (*ProviderTemplate, error) {
	tm.mu.RLock()
	tmpl := tm.templates[id]
	tm.mu.RUnlock()

	if tmpl == nil {
		return nil, fmt.Errorf("provider template '%s' not found", id)
	}
	return tmpl, nil
}

// GetAllTemplates returns all templates
func (tm *TemplateManager) GetAllTemplates() map[string]*ProviderTemplate {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	// Return a copy to avoid concurrent modification
	result := make(map[string]*ProviderTemplate, len(tm.templates))
	for k, v := range tm.templates {
		result[k] = v
	}
	return result
}

// GetVersion returns the current template version
func (tm *TemplateManager) GetVersion() string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.version
}

// FetchFromGitHub fetches templates from GitHub and updates the storage
func (tm *TemplateManager) FetchFromGitHub(ctx context.Context) (*ProviderTemplateRegistry, error) {
	// If no GitHub URL is configured, return error immediately
	if tm.githubURL == "" {
		return nil, fmt.Errorf("no GitHub URL configured")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", tm.githubURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add conditional request with ETag if available
	tm.etagMu.RLock()
	if tm.etag != "" {
		req.Header.Set("If-None-Match", tm.etag)
	}
	tm.etagMu.RUnlock()

	resp, err := tm.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch from GitHub: %w", err)
	}
	defer resp.Body.Close()

	// Handle 304 Not Modified
	if resp.StatusCode == http.StatusNotModified {
		// Return current state without modification
		tm.mu.RLock()
		providers := make(map[string]*ProviderTemplate, len(tm.templates))
		for k, v := range tm.templates {
			providers[k] = v
		}
		version := tm.version
		lastUpdated := tm.lastUpdated
		tm.mu.RUnlock()

		return &ProviderTemplateRegistry{
			Providers:   providers,
			Version:     version,
			LastUpdated: lastUpdated.Format(time.RFC3339),
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub returned status %d: %s", resp.StatusCode, string(body))
	}

	// Update ETag
	if etag := resp.Header.Get("ETag"); etag != "" {
		tm.etagMu.Lock()
		tm.etag = etag
		tm.etagMu.Unlock()
	}

	// Parse response
	var registry ProviderTemplateRegistry
	if err := json.NewDecoder(resp.Body).Decode(&registry); err != nil {
		return nil, fmt.Errorf("failed to parse registry JSON: %w", err)
	}

	// Update templates storage
	tm.mu.Lock()
	tm.templates = registry.Providers
	tm.capabilitySchemas = registry.CapabilitySchemas
	tm.lastUpdated = time.Now()
	tm.version = registry.Version
	tm.mu.Unlock()

	return &registry, nil
}

// TemplateCacheData represents the cache file structure
type TemplateCacheData struct {
	Registry ProviderTemplateRegistry `json:"registry"`
	CachedAt time.Time                `json:"cached_at"`
	Version  string                   `json:"version"`
	ETag     string                   `json:"etag,omitempty"`
}

// loadCache loads templates from cache file if valid
func (tm *TemplateManager) loadCache() (*ProviderTemplateRegistry, error) {
	cacheFile := filepath.Join(tm.cachePath, TemplateCacheFileName)

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Cache doesn't exist, not an error
		}
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}

	var cacheData TemplateCacheData
	if err := json.Unmarshal(data, &cacheData); err != nil {
		return nil, fmt.Errorf("failed to parse cache file: %w", err)
	}

	// Check if cache is still valid
	if time.Since(cacheData.CachedAt) > tm.cacheTTL {
		return nil, nil // Cache expired
	}

	// Restore ETag
	if cacheData.ETag != "" {
		tm.etagMu.Lock()
		tm.etag = cacheData.ETag
		tm.etagMu.Unlock()
	}

	return &cacheData.Registry, nil
}

// saveCache saves the current templates to cache file
func (tm *TemplateManager) saveCache(registry *ProviderTemplateRegistry) error {
	cacheFile := filepath.Join(tm.cachePath, TemplateCacheFileName)

	tm.mu.RLock()
	etag := tm.etag
	tm.mu.RUnlock()

	cacheData := TemplateCacheData{
		Registry: *registry,
		CachedAt: time.Now(),
		Version:  registry.Version,
		ETag:     etag,
	}

	data, err := json.MarshalIndent(cacheData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache data: %w", err)
	}

	// Write to temp file first, then rename for atomicity
	tmpFile := cacheFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	if err := os.Rename(tmpFile, cacheFile); err != nil {
		os.Remove(tmpFile) // Clean up temp file
		return fmt.Errorf("failed to rename cache file: %w", err)
	}

	return nil
}

// Initialize loads templates according to the source preference:
// - PreferenceDefault: Cache -> GitHub -> Embedded
// - PreferenceEmbedded: Embedded only (no network requests)
// - PreferenceEmbeddedFirst: Embedded -> Cache -> GitHub
func (tm *TemplateManager) Initialize(ctx context.Context) error {
	// First, always load embedded templates as immutable fallback
	if err := tm.loadEmbeddedTemplates(); err != nil {
		return err
	}

	switch tm.sourcePreference {
	case PreferenceEmbedded:
		// Use embedded templates only, skip all network requests
		tm.sourceMu.Lock()
		tm.source = TemplateSourceLocal
		tm.sourceMu.Unlock()
		return nil

	case PreferenceEmbeddedFirst:
		// Embedded is already loaded, return immediately
		// User can manually refresh from GitHub if needed
		tm.sourceMu.Lock()
		tm.source = TemplateSourceLocal
		tm.sourceMu.Unlock()
		return nil

	case PreferenceDefault:
		fallthrough
	default:
		// Try cache first (fastest, avoids network I/O)
		if tm.githubURL != "" {
			cachedRegistry, err := tm.loadCache()
			if err == nil && cachedRegistry != nil {
				// Cache hit - use cached templates
				tm.mu.Lock()
				tm.templates = cachedRegistry.Providers
				tm.lastUpdated = time.Now()
				tm.version = cachedRegistry.Version
				tm.mu.Unlock()

				tm.sourceMu.Lock()
				tm.source = TemplateSourceGitHub // Loaded from cache, but originally from GitHub
				tm.sourceMu.Unlock()
				return nil
			}
			// Cache miss or expired - try GitHub
			registry, err := tm.FetchFromGitHub(ctx)
			if err == nil {
				// GitHub fetch successful - save to cache
				_ = tm.saveCache(registry) // Ignore save errors, we have the data

				tm.sourceMu.Lock()
				tm.source = TemplateSourceGitHub
				tm.sourceMu.Unlock()
				return nil
			}
			// GitHub fetch failed, templates already has embedded fallback
		}

		// Using embedded templates
		tm.sourceMu.Lock()
		tm.source = TemplateSourceLocal
		tm.sourceMu.Unlock()
		return nil
	}
}

// loadEmbeddedTemplates loads templates from embedded JSON file into both templates and embedded
func (tm *TemplateManager) loadEmbeddedTemplates() error {
	var registry ProviderTemplateRegistry
	if err := json.Unmarshal(embeddedTemplatesJSON, &registry); err != nil {
		return fmt.Errorf("failed to parse embedded templates: %w", err)
	}

	// Make a deep copy for embedded (immutable fallback)
	embeddedCopy := make(map[string]*ProviderTemplate, len(registry.Providers))
	for k, v := range registry.Providers {
		embeddedCopy[k] = deepCopyTemplate(v)
	}

	// Also make a deep copy of capability schemas
	embeddedSchemas := make(map[string]*CapabilitySchema, len(registry.CapabilitySchemas))
	for k, v := range registry.CapabilitySchemas {
		embeddedSchemas[k] = deepCopyCapabilitySchema(v)
	}

	tm.mu.Lock()
	tm.embedded = embeddedCopy
	tm.embeddedSchemas = embeddedSchemas
	tm.templates = registry.Providers
	tm.capabilitySchemas = registry.CapabilitySchemas
	tm.lastUpdated = time.Now()
	tm.version = registry.Version
	tm.mu.Unlock()

	return nil
}

// ValidateTemplate validates a provider template
func ValidateTemplate(tmpl *ProviderTemplate) error {
	if tmpl.ID == "" {
		return fmt.Errorf("template ID is required")
	}
	if tmpl.Name == "" {
		return fmt.Errorf("template name is required")
	}
	// OAuth templates (auth_type == "oauth") don't require base_url
	// Non-OAuth templates must have at least one base URL
	if tmpl.AuthType != "oauth" && tmpl.BaseURLOpenAI == "" && tmpl.BaseURLAnthropic == "" {
		return fmt.Errorf("at least one base URL is required for non-OAuth templates")
	}
	// OAuth templates must have oauth_provider field set
	if tmpl.AuthType == "oauth" && tmpl.OAuthProvider == "" {
		return fmt.Errorf("oauth_provider is required for OAuth templates")
	}
	return nil
}

// findTemplateByProvider finds a matching template for the given provider.
// For OAuth providers, it matches by OAuthDetail.APIStyle against template.OAuthProvider.
// For API key providers, it matches by APIBase against template base URLs based on APIStyle.
func (tm *TemplateManager) findTemplateByProvider(provider *typ.Provider) *ProviderTemplate {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	// OAuth providers: match by OAuthProvider only, no fallback
	if provider.AuthType == typ.AuthTypeOAuth && provider.OAuthDetail != nil {
		oauthProviderType := provider.OAuthDetail.ProviderType
		return tm.searchTemplates(func(tmpl *ProviderTemplate) bool {
			return tmpl.OAuthProvider == oauthProviderType
		})
	}

	// API key providers: match by APIBase based on APIStyle
	apiBase := provider.APIBase
	// BUGFIX: ignore all "/" in right to make it consistent
	apiBase = strings.TrimRight(apiBase, "/")
	if apiBase == "" {
		return nil
	}

	// Determine which base URL field to match based on APIStyle
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		return tm.searchTemplates(func(tmpl *ProviderTemplate) bool {
			return tmpl.BaseURLAnthropic == apiBase
		})
	case protocol.APIStyleOpenAI:
		fallthrough
	default:
		return tm.searchTemplates(func(tmpl *ProviderTemplate) bool {
			return tmpl.BaseURLOpenAI == apiBase
		})
	}
}

// searchTemplates searches for a template by matcher function in both templates and embedded maps
func (tm *TemplateManager) searchTemplates(matcher func(*ProviderTemplate) bool) *ProviderTemplate {
	// Search in current templates first
	for _, tmpl := range tm.templates {
		if matcher(tmpl) {
			return tmpl
		}
	}
	// Search in embedded templates
	for _, tmpl := range tm.embedded {
		if matcher(tmpl) {
			return tmpl
		}
	}
	return nil
}

// deepCopyTemplate creates a deep copy of a ProviderTemplate
func deepCopyTemplate(tmpl *ProviderTemplate) *ProviderTemplate {
	result := *tmpl

	// Copy models slice
	if tmpl.Models != nil {
		result.Models = make([]string, len(tmpl.Models))
		copy(result.Models, tmpl.Models)
	}

	// Copy model limits map
	if tmpl.ModelLimits != nil {
		result.ModelLimits = make(map[string]int, len(tmpl.ModelLimits))
		for k, v := range tmpl.ModelLimits {
			result.ModelLimits[k] = v
		}
	}

	// Copy metadata map
	if tmpl.Metadata != nil {
		result.Metadata = make(map[string]string, len(tmpl.Metadata))
		for k, v := range tmpl.Metadata {
			result.Metadata[k] = v
		}
	}

	// Copy tags slice
	if tmpl.Tags != nil {
		result.Tags = make([]string, len(tmpl.Tags))
		copy(result.Tags, tmpl.Tags)
	}

	return &result
}

// deepCopyCapabilitySchema creates a deep copy of a CapabilitySchema
func deepCopyCapabilitySchema(schema *CapabilitySchema) *CapabilitySchema {
	result := *schema

	// Deep copy ResultFormat.Structure if it exists
	if schema.ResultFormat.Structure != nil {
		result.ResultFormat.Structure = make(map[string]interface{})
		for k, v := range schema.ResultFormat.Structure {
			result.ResultFormat.Structure[k] = v
		}
	}

	return &result
}

// GetModelsForProvider returns models for a provider using template-only hierarchy:
// 1. GitHub/embedded templates with models list
// Note: API-based model fetching is now handled by the client layer (client.ModelLister)
// This method only returns static models from templates
func (tm *TemplateManager) GetModelsForProvider(provider *typ.Provider) ([]string, TemplateSource, error) {
	// Find template by matching APIBase or OAuthProvider
	tmpl := tm.findTemplateByProvider(provider)

	if tmpl == nil {
		return nil, TemplateSourceLocal, fmt.Errorf("no matching template found for provider with api_base '%s'", provider.APIBase)
	}

	// Get source info
	tm.mu.RLock()
	source := tm.source
	tm.mu.RUnlock()

	// Return models from template
	if len(tmpl.Models) > 0 {
		return tmpl.Models, source, nil
	}

	return nil, TemplateSourceLocal, fmt.Errorf("no models found for provider with api_base '%s'", provider.APIBase)
}

// GetMaxTokensForModel returns the maximum allowed tokens for a specific model
// using the provider templates. If templates are not available, falls back to
// the global default.
// It checks in order:
// 1. Exact match of provider:model in templates
// 2. Model wildcard match (provider:*) in templates
// 3. Global default
func (tm *TemplateManager) GetMaxTokensForModel(provider, model string) int {
	// Try templates first if available
	if tm != nil {
		tmpl, _ := tm.GetTemplate(provider)
		if tmpl != nil && tmpl.ModelLimits != nil {
			// Check exact model match
			if maxTokens, ok := tmpl.ModelLimits[model]; ok {
				return maxTokens
			}
			// Check provider wildcard (provider:*)
			if maxTokens, ok := tmpl.ModelLimits[provider+":*"]; ok {
				return maxTokens
			}
		}
	}

	// Fallback to global default
	return constant.DefaultMaxTokens
}

// GetMaxTokensForModelByProvider returns the maximum allowed tokens for a specific model
// using the provider templates matched by APIBase or OAuthProvider.
// This is the preferred method as it correctly matches templates regardless of user-defined provider name.
func (tm *TemplateManager) GetMaxTokensForModelByProvider(provider *typ.Provider, model string) int {
	if tm == nil || provider == nil {
		return constant.DefaultMaxTokens
	}

	// Find matching template
	tmpl := tm.findTemplateByProvider(provider)
	if tmpl != nil && tmpl.ModelLimits != nil {
		// Check exact model match
		if maxTokens, ok := tmpl.ModelLimits[model]; ok {
			return maxTokens
		}
	}

	// Fallback to global default
	return constant.DefaultMaxTokens
}

// GetWebSearchSchemaForProvider returns the web search capability schema for a provider
// Returns nil if the provider doesn't have web_search_schema defined or the schema doesn't exist
func (tm *TemplateManager) GetWebSearchSchemaForProvider(provider *typ.Provider) *CapabilitySchema {
	if tm == nil || provider == nil {
		return nil
	}

	// Find matching template
	tmpl := tm.findTemplateByProvider(provider)
	if tmpl == nil || tmpl.WebSearchSchema == "" {
		return nil
	}

	// Get the schema from the registry
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	// Check current capability schemas
	if schema, ok := tm.capabilitySchemas[tmpl.WebSearchSchema]; ok {
		return schema
	}

	// Fallback to embedded capability schemas
	if schema, ok := tm.embeddedSchemas[tmpl.WebSearchSchema]; ok {
		return schema
	}

	return nil
}
