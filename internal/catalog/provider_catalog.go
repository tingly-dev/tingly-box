package catalog

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

//go:embed providers.json
var embeddedTemplatesJSON []byte

const DefaultCatalogHTTPTimeout = 30 * time.Second // Default HTTP timeout for fetching templates

const DefaultCatalogCacheTTL = 12 * time.Hour // Default TTL for template cache

const CatalogCacheFileName = "provider_catalog.json"

const CatalogGitHubURL = "https://raw.githubusercontent.com/tingly-dev/tingly-box/main/internal/catalog/providers.json"

// ModelInfo represents detailed information about a model
type ModelInfo struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	Context     int    `json:"context,omitempty"`
	MaxOutput   int    `json:"max_output,omitempty"`

	// OpenAIEndpoints declares which OpenAI endpoint(s) THIS model supports,
	// for a template whose catalog mixes vendors (OpenCode Zen: most models
	// are Chat-only, a few are Responses-only, and some may support both) —
	// a fact ProviderCatalog.OpenAIEndpointMode can't express since it's one
	// value per provider. A list rather than a single value so a model that
	// answers on both endpoints can say so, instead of only ever declaring
	// one. Empty means no declaration for this model: falls through exactly
	// as if this field didn't exist (defers to the provider default, which
	// resolves to Chat unless the provider itself declares otherwise). Static
	// and hand-maintained: an unlisted model just falls through to the
	// provider default (see .design/openai-endpoint-routing.md §10). Values:
	// any subset of "chat", "responses"; unrecognized entries are ignored so
	// a typo degrades to "no declaration" rather than a wrong route.
	OpenAIEndpoints []string `json:"openai_endpoints,omitempty"`
}

// NamingRules defines the naming conventions for provider IDs
type NamingRules struct {
	KeyFormat      string `json:"key_format"`
	SlugRule1      string `json:"slug_rule_1,omitempty"`
	SlugRule2      string `json:"slug_rule_2,omitempty"`
	SlugRule3      string `json:"slug_rule_3,omitempty"`
	Variant        string `json:"variant,omitempty"`
	RequiredFields struct {
		CanonicalDomain string `json:"canonical_domain"`
		VendorFamily    string `json:"vendor_family"`
		Region          string `json:"region"`
		Plan            string `json:"plan"`
		ModelsSchema    string `json:"models_schema"`
	} `json:"required_fields"`
}
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

// ProviderCatalog represents a predefined provider configuration template
type ProviderCatalog struct {
	// Core identification
	ID      string `json:"id"`
	Name    string `json:"name"`
	Alias   string `json:"alias,omitempty"` // Display name with locale information
	Status  string `json:"status"`          // "active", "deprecated", etc.
	Valid   bool   `json:"valid"`
	Website string `json:"website"`

	// NEW: Structured metadata (Schema V2)
	CanonicalDomain string `json:"canonical_domain"` // API host (e.g., "api.anthropic.com")
	VendorFamily    string `json:"vendor_family"`    // Vendor aggregation key (e.g., "anthropic", "alibaba")
	Region          string `json:"region"`           // "cn" | "intl" | "global"
	Plan            string `json:"plan"`             // "standard" | "coding" | "oauth"

	Description string `json:"description"`

	// Documentation
	APIDoc     string `json:"api_doc"`
	ModelDoc   string `json:"model_doc"`
	PricingDoc string `json:"pricing_doc"`

	// API endpoints
	BaseURLOpenAI    string `json:"base_url_openai,omitempty"`
	BaseURLAnthropic string `json:"base_url_anthropic,omitempty"`

	// APIStyle explicitly declares the provider protocol ("openai" | "anthropic"
	// | "google"). Optional: when empty the style is inferred from which base URL
	// is set. Cloud templates that share a canonical_domain but differ by model
	// family (Vertex Claude vs Vertex Gemini) set this so template matching can
	// disambiguate them by the provider's APIStyle.
	APIStyle string `json:"api_style,omitempty"`

	// CHANGED: Models now include context/max_output directly (Schema V2)
	Models []ModelInfo `json:"models"`

	SupportsModelsEndpoint bool `json:"supports_models_endpoint"`

	// Authentication
	AuthType      string `json:"auth_type,omitempty"`      // "oauth", "key"
	OAuthProvider string `json:"oauth_provider,omitempty"` // OAuth provider type for oauth type providers

	// Capabilities
	WebSearchSchema string `json:"web_search_schema,omitempty"` // Reference to capability schema for web_search
	Icon            string `json:"icon,omitempty"`              // Icon identifier (e.g., "openai", "anthropic") for Lobe Icons

	// Capacity configuration for TacticCapacityBased load balancing
	// TotalCapacity is the total seat count for this provider (shared across all models)
	TotalCapacity *int `json:"total_capacity,omitempty"`
	// DefaultModelCapacity is the default seat count per model (template)
	// Each model inherits this unless overridden
	DefaultModelCapacity *int `json:"default_model_capacity,omitempty"`
	// ModelCapacities allows per-model capacity overrides
	ModelCapacities map[string]int `json:"model_capacities,omitempty"` // model name -> capacity

	// OpenAIEndpointMode declares which OpenAI endpoints providers instantiated
	// from this template expose. Plain string at this layer; cast to the typed
	// ai.OpenAIEndpointMode when assigned to a Provider. Values: "" (Chat,
	// default), "responses" (Codex-style), "both" (OpenAI proper).
	OpenAIEndpointMode string `json:"openai_endpoint_mode,omitempty"`
}

// ProviderCatalogRegistry represents the provider template registry structure from GitHub
type ProviderCatalogRegistry struct {
	SchemaVersion     int                          `json:"_schema_version"`
	NamingRules       *NamingRules                 `json:"_naming_rules,omitempty"`
	Providers         map[string]*ProviderCatalog  `json:"providers"`
	CapabilitySchemas map[string]*CapabilitySchema `json:"capability_schemas,omitempty"`
	Version           string                       `json:"version"`
	LastUpdated       string                       `json:"last_updated"`
}

// CatalogSource tracks where templates were loaded from
type CatalogSource int

const (
	// CatalogSourceGitHub - From GitHub templates
	CatalogSourceGitHub CatalogSource = iota
	// CatalogSourceLocal - From local embedded templates
	CatalogSourceLocal
)

// CatalogSourcePreference defines the priority order for loading templates
type CatalogSourcePreference int

const (
	// PreferenceDefault: Cache -> GitHub -> Embedded
	PreferenceDefault CatalogSourcePreference = iota
	// PreferenceEmbedded: Embedded only (no network requests)
	PreferenceEmbedded
	// PreferenceEmbeddedFirst: Embedded -> Cache -> GitHub
	PreferenceEmbeddedFirst
)

// ProviderCatalogManager manages provider templates with -tier fallback
type ProviderCatalogManager struct {
	templates         map[string]*ProviderCatalog  // Current templates from GitHub or embedded
	embedded          map[string]*ProviderCatalog  // Embedded templates (immutable fallback)
	capabilitySchemas map[string]*CapabilitySchema // Current capability schemas
	embeddedSchemas   map[string]*CapabilitySchema // Embedded capability schemas
	mu                sync.RWMutex
	lastUpdated       time.Time     // Last update timestamp
	version           string        // Template version
	source            CatalogSource // Current source: GitHub or Local
	sourceMu          sync.RWMutex
	etag              string // For conditional GitHub requests
	etagMu            sync.RWMutex
	githubURL         string                  // Empty means no GitHub sync, only embedded templates
	sourcePreference  CatalogSourcePreference // Priority order for loading templates
	httpClient        *http.Client
	cachePath         string        // Path to cache file
	cacheTTL          time.Duration // Cache TTL (default 24h)
}

func NewDefaultProviderCatalogManager() *ProviderCatalogManager {
	return NewProviderCatalogManagerWithPreference(CatalogGitHubURL, PreferenceDefault)
}

// NewEmbeddedOnlyProviderCatalogManager creates a template manager that only uses embedded templates
// This is useful for development, testing, or offline scenarios
func NewEmbeddedOnlyProviderCatalogManager() *ProviderCatalogManager {
	return NewProviderCatalogManagerWithPreference("", PreferenceEmbedded)
}

// NewProviderCatalogManager creates a new template manager with default preference.
// If githubURL is empty, only embedded templates will be used (no GitHub sync).
func NewProviderCatalogManager(githubURL string) *ProviderCatalogManager {
	return NewProviderCatalogManagerWithPreference(githubURL, PreferenceDefault)
}

// NewProviderCatalogManagerWithPreference creates a new template manager with specified source preference.
// If githubURL is empty, only embedded templates will be used (no GitHub sync).
func NewProviderCatalogManagerWithPreference(githubURL string, preference CatalogSourcePreference) *ProviderCatalogManager {
	configDir := constant.GetTinglyConfDir()
	return &ProviderCatalogManager{
		githubURL:         githubURL,
		sourcePreference:  preference,
		templates:         make(map[string]*ProviderCatalog),
		capabilitySchemas: make(map[string]*CapabilitySchema),
		httpClient: &http.Client{
			Timeout: DefaultCatalogHTTPTimeout,
		},
		cachePath: configDir, // Will store in .tingly-box directory
		cacheTTL:  DefaultCatalogCacheTTL,
	}
}

// GetTemplate returns a provider template by ID
func (tm *ProviderCatalogManager) GetTemplate(id string) (*ProviderCatalog, error) {
	tm.mu.RLock()
	tmpl := tm.templates[id]
	tm.mu.RUnlock()

	if tmpl == nil {
		return nil, fmt.Errorf("provider template '%s' not found", id)
	}
	return tmpl, nil
}

// GetAllTemplates returns all templates
func (tm *ProviderCatalogManager) GetAllTemplates() map[string]*ProviderCatalog {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	// Return a copy to avoid concurrent modification
	result := make(map[string]*ProviderCatalog, len(tm.templates))
	maps.Copy(result, tm.templates)
	return result
}

// GetVersion returns the current template version
func (tm *ProviderCatalogManager) GetVersion() string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.version
}

// FetchTemplates fetches templates from URL (http://, https://, or file://)
func (tm *ProviderCatalogManager) FetchTemplates(ctx context.Context) (*ProviderCatalogRegistry, error) {
	if tm.githubURL == "" {
		return nil, fmt.Errorf("no template source configured")
	}

	if strings.HasPrefix(tm.githubURL, "file://") {
		return tm.fetchFromFile(tm.githubURL[7:])
	}

	return tm.fetchFromHTTP(ctx)
}

// setExternalTemplates is the single write seam for externally sourced
// registries (GitHub, its disk cache, file://). It merges embedded-only
// templates in (see mergeEmbeddedOnly) so every ingestion path preserves the
// "templates ⊇ embedded ids" invariant, and only adopts capability schemas
// when the external source actually carries some.
func (tm *ProviderCatalogManager) setExternalTemplates(registry *ProviderCatalogRegistry) {
	tm.mu.Lock()
	tm.templates = tm.mergeEmbeddedOnly(registry.Providers)
	if registry.CapabilitySchemas != nil {
		tm.capabilitySchemas = registry.CapabilitySchemas
	}
	tm.version = registry.Version
	tm.lastUpdated = time.Now()
	tm.mu.Unlock()
}

// fetchFromFile loads templates from a local file
func (tm *ProviderCatalogManager) fetchFromFile(filePath string) (*ProviderCatalogRegistry, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template file: %w", err)
	}

	var registry ProviderCatalogRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("failed to parse template JSON: %w", err)
	}

	tm.setExternalTemplates(&registry)
	tm.sourceMu.Lock()
	tm.source = CatalogSourceLocal
	tm.sourceMu.Unlock()

	return &registry, nil
}

// fetchFromHTTP fetches templates from HTTP/HTTPS URL
func (tm *ProviderCatalogManager) fetchFromHTTP(ctx context.Context) (*ProviderCatalogRegistry, error) {
	// If no URL is configured, return error immediately
	if tm.githubURL == "" {
		return nil, fmt.Errorf("no template URL configured")
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
		providers := make(map[string]*ProviderCatalog, len(tm.templates))
		maps.Copy(providers, tm.templates)
		version := tm.version
		lastUpdated := tm.lastUpdated
		tm.mu.RUnlock()

		return &ProviderCatalogRegistry{
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
	var registry ProviderCatalogRegistry
	if err := json.NewDecoder(resp.Body).Decode(&registry); err != nil {
		return nil, fmt.Errorf("failed to parse registry JSON: %w", err)
	}

	// Persist the pure remote registry before merging: the cache must never
	// contain this binary's embedded entries, or a later binary's embedded
	// fixes would lose to them until the cache expires.
	_ = tm.saveCache(&registry)

	tm.setExternalTemplates(&registry)

	return &registry, nil
}

// CatalogCacheData represents the cache file structure
type CatalogCacheData struct {
	Registry ProviderCatalogRegistry `json:"registry"`
	CachedAt time.Time               `json:"cached_at"`
	Version  string                  `json:"version"`
	ETag     string                  `json:"etag,omitempty"`
}

// loadCache loads templates from cache file if valid
func (tm *ProviderCatalogManager) loadCache() (*ProviderCatalogRegistry, error) {
	cacheFile := filepath.Join(tm.cachePath, CatalogCacheFileName)

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Cache doesn't exist, not an error
		}
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}

	var cacheData CatalogCacheData
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
func (tm *ProviderCatalogManager) saveCache(registry *ProviderCatalogRegistry) error {
	cacheFile := filepath.Join(tm.cachePath, CatalogCacheFileName)

	tm.etagMu.RLock()
	etag := tm.etag
	tm.etagMu.RUnlock()

	cacheData := CatalogCacheData{
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
func (tm *ProviderCatalogManager) Initialize(ctx context.Context) error {
	// First, always load embedded templates as immutable fallback
	if err := tm.loadEmbeddedTemplates(); err != nil {
		return err
	}

	switch tm.sourcePreference {
	case PreferenceEmbedded:
		// Use embedded templates only, skip all network requests
		tm.sourceMu.Lock()
		tm.source = CatalogSourceLocal
		tm.sourceMu.Unlock()
		return nil

	case PreferenceEmbeddedFirst:
		// Embedded is already loaded, return immediately
		// User can manually refresh from GitHub if needed
		tm.sourceMu.Lock()
		tm.source = CatalogSourceLocal
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
				tm.setExternalTemplates(cachedRegistry)

				tm.sourceMu.Lock()
				tm.source = CatalogSourceGitHub // Loaded from cache, but originally from GitHub
				tm.sourceMu.Unlock()
				return nil
			}
			// Cache miss or expired - try GitHub. fetchFromHTTP persists the
			// pure remote registry to the cache itself.
			_, err = tm.FetchTemplates(ctx)
			if err == nil {
				tm.sourceMu.Lock()
				tm.source = CatalogSourceGitHub
				tm.sourceMu.Unlock()
				return nil
			}
			// GitHub fetch failed, templates already has embedded fallback
		}

		// Using embedded templates
		tm.sourceMu.Lock()
		tm.source = CatalogSourceLocal
		tm.sourceMu.Unlock()
		return nil
	}
}

// mergeEmbeddedOnly returns a new map combining an externally sourced set
// (GitHub registry or its disk cache) with this binary's embedded templates.
// External entries win on id collision, but templates that ship only with this
// binary — e.g. the cloud presets a new feature depends on — must stay visible
// even when the remote registry predates them. The input is not mutated: the
// external set (and hence the disk cache built from it) stays pure remote
// content, so a newer binary's embedded fixes are never shadowed by embedded
// values a previous binary laundered into the cache. Callers must hold tm.mu.
func (tm *ProviderCatalogManager) mergeEmbeddedOnly(external map[string]*ProviderCatalog) map[string]*ProviderCatalog {
	merged := make(map[string]*ProviderCatalog, len(external)+len(tm.embedded))
	for id, tmpl := range tm.embedded {
		merged[id] = deepCopyTemplate(tmpl)
	}
	maps.Copy(merged, external)
	return merged
}

// loadEmbeddedTemplates loads templates from embedded JSON file into both templates and embedded
func (tm *ProviderCatalogManager) loadEmbeddedTemplates() error {
	var registry ProviderCatalogRegistry
	if err := json.Unmarshal(embeddedTemplatesJSON, &registry); err != nil {
		return fmt.Errorf("failed to parse embedded templates: %w", err)
	}

	// Make a deep copy for embedded (immutable fallback)
	embeddedCopy := make(map[string]*ProviderCatalog, len(registry.Providers))
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

// ValidateProviderCatalog validates a provider template
func ValidateProviderCatalog(tmpl *ProviderCatalog) error {
	if tmpl.ID == "" {
		return fmt.Errorf("template ID is required")
	}
	if tmpl.Name == "" {
		return fmt.Errorf("template name is required")
	}
	// OAuth templates (auth_type == "oauth") and cloud multi-field credential
	// templates (aws_sigv4/gcp_sa/azure_key) don't require a base URL — their
	// endpoint is derived from the credential (region/project/endpoint) at
	// connect time. Every other template must carry at least one base URL.
	isCloud := typ.AuthType(tmpl.AuthType).IsMultiFieldCredential()
	if tmpl.AuthType != "oauth" && !isCloud && tmpl.BaseURLOpenAI == "" && tmpl.BaseURLAnthropic == "" {
		return fmt.Errorf("at least one base URL is required for non-OAuth templates")
	}
	// OAuth templates must have oauth_provider field set
	if tmpl.AuthType == "oauth" && tmpl.OAuthProvider == "" {
		return fmt.Errorf("oauth_provider is required for OAuth templates")
	}
	return nil
}

// findTemplateByProvider finds a matching template for the given provider,
// searching the active (possibly remote) template set with embedded fallback.
// OAuth providers match by OAuthDetail.Issuer; multi-field cloud providers by
// auth_type + api_style; API-key providers by APIBase against canonical_domain
// or base URLs.
func (tm *ProviderCatalogManager) findTemplateByProvider(provider *typ.Provider) *ProviderCatalog {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return matchProviderTemplate(provider, tm.searchTemplates)
}

// matchProviderTemplate holds the provider→template matching rules once; the
// search parameter selects the template set (active vs embedded-only).
func matchProviderTemplate(provider *typ.Provider, search func(func(*ProviderCatalog) bool) *ProviderCatalog) *ProviderCatalog {
	// OAuth providers: match by OAuthProvider only, no fallback
	if provider.IsOAuth() && provider.OAuthDetail != nil {
		issuer := provider.OAuthDetail.Issuer
		return search(func(tmpl *ProviderCatalog) bool {
			return tmpl.OAuthProvider == string(issuer)
		})
	}

	// Multi-field cloud providers (Bedrock/Vertex/Azure): match by identity —
	// auth_type plus api_style — not by URL. The credential-derived host varies
	// by region, and Vertex multi-regional hosts (aiplatform.us.rep.googleapis.com)
	// don't even contain the canonical domain.
	if provider.IsMultiFieldCredential() {
		return search(cloudTemplateMatcher(provider))
	}

	// API key providers: match by APIBase based on APIStyle
	// BUGFIX: ignore all "/" in right to make it consistent
	apiBase := strings.TrimRight(provider.APIBase, "/")
	if apiBase == "" {
		return nil
	}

	// Try matching by canonical_domain first (Schema V2). When a template
	// declares an explicit APIStyle it must also match the provider's style so
	// the right model family is chosen.
	//
	// More than one template can share a canonical_domain (OpenCode Zen:
	// "opencode-ai" at /zen and "opencode-go" at /zen/go, neither setting
	// api_style). Collect every match and prefer the one whose own base URL is
	// the longest prefix of the provider's, rather than the first hit off the
	// map — the latter picked a random one of the two on every process
	// restart. A single match keeps the original behavior exactly.
	var candidates []*ProviderCatalog
	search(func(tmpl *ProviderCatalog) bool {
		if tmpl.CanonicalDomain != "" && strings.Contains(apiBase, tmpl.CanonicalDomain) &&
			(tmpl.APIStyle == "" || tmpl.APIStyle == string(provider.APIStyle)) {
			candidates = append(candidates, tmpl)
		}
		return false // keep scanning; never let search() short-circuit on first hit
	})
	if len(candidates) == 1 {
		return candidates[0]
	}
	if len(candidates) > 1 {
		return mostSpecificTemplate(candidates, apiBase)
	}

	// Fallback: Determine which base URL field to match based on APIStyle
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		return search(func(tmpl *ProviderCatalog) bool {
			return tmpl.BaseURLAnthropic == apiBase
		})
	default:
		return search(func(tmpl *ProviderCatalog) bool {
			return tmpl.BaseURLOpenAI == apiBase
		})
	}
}

// mostSpecificTemplate picks the template whose declared base URL is the
// longest prefix of apiBase, among templates that matched on canonical_domain
// alone. Falls back to the first candidate if none qualifies (malformed
// template data — not expected in practice).
func mostSpecificTemplate(candidates []*ProviderCatalog, apiBase string) *ProviderCatalog {
	best := candidates[0]
	bestLen := -1
	for _, tmpl := range candidates {
		for _, base := range []string{tmpl.BaseURLOpenAI, tmpl.BaseURLAnthropic} {
			base = strings.TrimRight(base, "/")
			if base == "" || !strings.HasPrefix(apiBase, base) {
				continue
			}
			if len(base) > bestLen {
				bestLen = len(base)
				best = tmpl
			}
		}
	}
	return best
}

// cloudTemplateMatcher matches a multi-field cloud provider to its template by
// auth_type, with api_style disambiguating templates that share an auth type
// (Vertex Claude vs Gemini).
func cloudTemplateMatcher(provider *typ.Provider) func(*ProviderCatalog) bool {
	return func(tmpl *ProviderCatalog) bool {
		return tmpl.AuthType == string(provider.AuthType) &&
			(tmpl.APIStyle == "" || tmpl.APIStyle == string(provider.APIStyle))
	}
}

// searchTemplates searches the active template set. Every ingestion path goes
// through setExternalTemplates (or loadEmbeddedTemplates), so tm.templates is
// always a superset of the embedded ids — no separate embedded scan is needed
// here. searchEmbedded exists for callers that deliberately bypass a possibly
// disk-cached set.
func (tm *ProviderCatalogManager) searchTemplates(matcher func(*ProviderCatalog) bool) *ProviderCatalog {
	for _, tmpl := range tm.templates {
		if matcher(tmpl) {
			return tmpl
		}
	}
	return nil
}

// searchEmbedded searches only tm.embedded, bypassing the (possibly disk-cached) tm.templates.
func (tm *ProviderCatalogManager) searchEmbedded(matcher func(*ProviderCatalog) bool) *ProviderCatalog {
	for _, tmpl := range tm.embedded {
		if matcher(tmpl) {
			return tmpl
		}
	}
	return nil
}

// deepCopyTemplate creates a deep copy of a ProviderCatalog
func deepCopyTemplate(tmpl *ProviderCatalog) *ProviderCatalog {
	result := *tmpl

	// Copy models slice (NEW: ModelInfo array)
	if tmpl.Models != nil {
		result.Models = make([]ModelInfo, len(tmpl.Models))
		copy(result.Models, tmpl.Models)
	}

	// Copy model capacities map
	if tmpl.ModelCapacities != nil {
		result.ModelCapacities = make(map[string]int, len(tmpl.ModelCapacities))
		maps.Copy(result.ModelCapacities, tmpl.ModelCapacities)
	}

	// Copy pointer scalars so the copy shares no memory with the original
	if tmpl.TotalCapacity != nil {
		v := *tmpl.TotalCapacity
		result.TotalCapacity = &v
	}
	if tmpl.DefaultModelCapacity != nil {
		v := *tmpl.DefaultModelCapacity
		result.DefaultModelCapacity = &v
	}

	return &result
}

// deepCopyCapabilitySchema creates a deep copy of a CapabilitySchema
func deepCopyCapabilitySchema(schema *CapabilitySchema) *CapabilitySchema {
	result := *schema

	// Deep copy ResultFormat.Structure if it exists
	if schema.ResultFormat.Structure != nil {
		result.ResultFormat.Structure = make(map[string]interface{})
		maps.Copy(result.ResultFormat.Structure, schema.ResultFormat.Structure)
	}

	return &result
}

// GetModelsForProvider returns models for a provider using template-only hierarchy:
// 1. GitHub/embedded templates with models list
// Note: API-based model fetching is now handled by the client layer (client.ModelLister)
// This method only returns static models from templates
func (tm *ProviderCatalogManager) GetModelsForProvider(provider *typ.Provider) ([]string, CatalogSource, error) {
	// Find template by matching APIBase or OAuthProvider
	tmpl := tm.findTemplateByProvider(provider)

	if tmpl == nil {
		return nil, CatalogSourceLocal, fmt.Errorf("no matching template found for provider with api_base '%s'", provider.APIBase)
	}

	// Get source info
	tm.mu.RLock()
	source := tm.source
	tm.mu.RUnlock()

	// NEW: Extract model IDs from ModelInfo array
	if len(tmpl.Models) > 0 {
		modelIDs := make([]string, len(tmpl.Models))
		for i, m := range tmpl.Models {
			modelIDs[i] = m.ID
		}
		return modelIDs, source, nil
	}

	return nil, CatalogSourceLocal, fmt.Errorf("no models found for provider with api_base '%s'", provider.APIBase)
}

// GetEmbeddedModelsForProvider returns models from the compile-time embedded providers.json,
// bypassing any disk cache. Use this for fallback paths where the provider API is unavailable,
// so the result is always the binary's built-in defaults rather than a potentially stale cache.
func (tm *ProviderCatalogManager) GetEmbeddedModelsForProvider(provider *typ.Provider) ([]string, error) {
	tmpl := tm.findEmbeddedTemplateByProvider(provider)
	if tmpl == nil {
		return nil, fmt.Errorf("no embedded template found for provider with api_base '%s'", provider.APIBase)
	}
	if len(tmpl.Models) == 0 {
		return nil, fmt.Errorf("no models in embedded template for provider with api_base '%s'", provider.APIBase)
	}
	modelIDs := make([]string, len(tmpl.Models))
	for i, m := range tmpl.Models {
		modelIDs[i] = m.ID
	}
	return modelIDs, nil
}

// findEmbeddedTemplateByProvider is like findTemplateByProvider but searches only tm.embedded,
// not the (possibly disk-cached) tm.templates.
func (tm *ProviderCatalogManager) findEmbeddedTemplateByProvider(provider *typ.Provider) *ProviderCatalog {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return matchProviderTemplate(provider, tm.searchEmbedded)
}

// GetMaxTokensForModel returns the maximum allowed tokens for a specific model
// using the provider templates. If templates are not available, falls back to
// the global default.
// It checks in order:
// 1. Exact match in Models array (ModelInfo.MaxOutput)
// 2. ModelCapacities override (for capacity-based limits)
// 3. Global default
func (tm *ProviderCatalogManager) GetMaxTokensForModel(provider, model string) int {
	// Try templates first if available
	if tm != nil {
		tmpl, _ := tm.GetTemplate(provider)
		if tmpl != nil {
			// NEW: Check Models array for MaxOutput
			for _, m := range tmpl.Models {
				if m.ID == model && m.MaxOutput > 0 {
					return m.MaxOutput
				}
			}
			// Fallback to ModelCapacities (for capacity-based limits)
			if tmpl.ModelCapacities != nil {
				if maxTokens, ok := tmpl.ModelCapacities[model]; ok {
					return maxTokens
				}
			}
		}
	}

	// Fallback to global default
	return constant.DefaultMaxTokens
}

// GetMaxTokensForModelByProvider returns the maximum allowed tokens for a specific model
// using the provider templates matched by APIBase or OAuthProvider.
// This is the preferred method as it correctly matches templates regardless of user-defined provider name.
func (tm *ProviderCatalogManager) GetMaxTokensForModelByProvider(provider *typ.Provider, model string) int {
	if tm == nil || provider == nil {
		return constant.DefaultMaxTokens
	}

	// Find matching template
	tmpl := tm.findTemplateByProvider(provider)
	if tmpl != nil {
		// NEW: Check Models array for MaxOutput
		for _, m := range tmpl.Models {
			if m.ID == model && m.MaxOutput > 0 {
				return m.MaxOutput
			}
		}
		// Fallback to ModelCapacities (for capacity-based limits)
		if tmpl.ModelCapacities != nil {
			if maxTokens, ok := tmpl.ModelCapacities[model]; ok {
				return maxTokens
			}
		}
	}

	// Fallback to global default
	return constant.DefaultMaxTokens
}

// GetOpenAIEndpointOverrideForModel looks up ModelInfo.OpenAIEndpoints for
// this model on this provider's template and returns the mode it declares.
// ai.EndpointModeUnknown means no declaration — resolve exactly as if this
// function didn't exist. Matched by template like
// GetMaxTokensForModelByProvider, so it works regardless of the provider's
// display name.
func (tm *ProviderCatalogManager) GetOpenAIEndpointOverrideForModel(provider *typ.Provider, model string) ai.OpenAIEndpointMode {
	if tm == nil || provider == nil || model == "" {
		return ai.EndpointModeUnknown
	}
	tmpl := tm.findTemplateByProvider(provider)
	if tmpl == nil {
		return ai.EndpointModeUnknown
	}
	for _, m := range tmpl.Models {
		if m.ID != model {
			continue
		}
		return openAIEndpointModeFromList(m.OpenAIEndpoints)
	}
	return ai.EndpointModeUnknown
}

// openAIEndpointModeFromList folds a ModelInfo.OpenAIEndpoints list into the
// single ai.OpenAIEndpointMode the resolver already understands. Unrecognized
// entries are ignored, so a typo in the data file degrades to "no
// declaration" for that entry rather than a wrong route.
func openAIEndpointModeFromList(endpoints []string) ai.OpenAIEndpointMode {
	var hasChat, hasResponses bool
	for _, e := range endpoints {
		switch e {
		case "chat":
			hasChat = true
		case "responses":
			hasResponses = true
		}
	}
	switch {
	case hasChat && hasResponses:
		return ai.EndpointModeBoth
	case hasResponses:
		return ai.EndpointModeResponses
	case hasChat:
		return ai.EndpointModeChat
	default:
		return ai.EndpointModeUnknown
	}
}

// GetWebSearchSchemaForProvider returns the web search capability schema for a provider
// Returns nil if the provider doesn't have web_search_schema defined or the schema doesn't exist
func (tm *ProviderCatalogManager) GetWebSearchSchemaForProvider(provider *typ.Provider) *CapabilitySchema {
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

// ProviderHasBuiltInWebSearch checks if a provider has built-in web_search capability
// Returns true if the provider has a web_search_schema with BuiltIn=true
func (tm *ProviderCatalogManager) ProviderHasBuiltInWebSearch(provider *typ.Provider) bool {
	if tm == nil || provider == nil {
		return false
	}

	schema := tm.GetWebSearchSchemaForProvider(provider)
	return schema != nil && schema.BuiltIn
}
