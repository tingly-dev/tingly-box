package typ

import (
	"time"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
)

// RuleScenario represents the scenario for a routing rule
type RuleScenario string

const (
	ScenarioOpenAI     RuleScenario = "openai"
	ScenarioAnthropic  RuleScenario = "anthropic"
	ScenarioClaudeCode RuleScenario = "claude_code"
	ScenarioOpenCode   RuleScenario = "opencode"
	ScenarioXcode      RuleScenario = "xcode"
	ScenarioGlobal     RuleScenario = "_global" // Global flags that apply to all scenarios
)

// ScenarioFlags represents configuration flags for a scenario
type ScenarioFlags struct {
	Unified  bool `json:"unified" yaml:"unified"`   // Single configuration for all models
	Separate bool `json:"separate" yaml:"separate"` // Separate configuration for each model
	Smart    bool `json:"smart" yaml:"smart"`       // Smart mode with automatic optimization

	// Experimental feature flags (scenario-based opt-in)
	SmartCompact bool `json:"smart_compact,omitempty" yaml:"smart_compact,omitempty"` // Enable smart compact (remove thinking blocks)
	Recording    bool `json:"recording,omitempty" yaml:"recording,omitempty"`         // Enable scenario recording (JSONL files)
	SmartMemory  bool `json:"smart_memory,omitempty" yaml:"smart_memory,omitempty"`   // Enable prompt recording (database)
}

// ScenarioConfig represents configuration for a specific scenario
type ScenarioConfig struct {
	Scenario   RuleScenario           `json:"scenario" yaml:"scenario"`
	Flags      ScenarioFlags          `json:"flags" yaml:"flags"`                               // Scenario configuration flags
	Extensions map[string]interface{} `json:"extensions,omitempty" yaml:"extensions,omitempty"` // Reserved for future extensions
}

// GetDefaultFlags returns default flags for a scenario
func (sc *ScenarioConfig) GetDefaultFlags() ScenarioFlags {
	if sc.Flags.Unified || sc.Flags.Separate || sc.Flags.Smart {
		return sc.Flags
	}
	// Default to unified if no flag is set
	return ScenarioFlags{Unified: true}
}

// AuthType represents the authentication type for a provider
type AuthType string

const (
	AuthTypeAPIKey AuthType = "api_key"
	AuthTypeOAuth  AuthType = "oauth"
)

// OAuthDetail contains OAuth-specific authentication information
type OAuthDetail struct {
	AccessToken  string                 `json:"access_token"`  // OAuth access token
	ProviderType string                 `json:"provider_type"` // anthropic, google, etc. for token manager lookup
	UserID       string                 `json:"user_id"`       // OAuth user identifier
	RefreshToken string                 `json:"refresh_token"` // Token for refreshing access token
	ExpiresAt    string                 `json:"expires_at"`    // Token expiration time (RFC3339)
	ExtraFields  map[string]interface{} `json:"extra_fields"`  // Any extra field for some special clients
}

// ToolInterceptorConfig contains configuration for tool interceptor (search & fetch)
type ToolInterceptorConfig struct {
	PreferLocalSearch bool `json:"prefer_local_search,omitempty"` // Prefer local tool interception even if provider has built-in search
	SearchAPI  string `json:"search_api,omitempty"`  // "brave" or "google"
	SearchKey  string `json:"search_key,omitempty"`  // API key for search service
	MaxResults int    `json:"max_results,omitempty"` // Max search results to return (default: 10)

	// Proxy configuration
	ProxyURL string `json:"proxy_url,omitempty"` // HTTP proxy URL (e.g., "http://127.0.0.1:7897")

	// Fetch configuration
	MaxFetchSize int64 `json:"max_fetch_size,omitempty"` // Max content size for fetch in bytes (default: 1MB)
	FetchTimeout int64 `json:"fetch_timeout,omitempty"`  // Fetch timeout in seconds (default: 30)
	MaxURLLength int   `json:"max_url_length,omitempty"` // Max URL length (default: 2000)
}

// ToolInterceptorOverride contains provider-level overrides for tool interceptor
type ToolInterceptorOverride struct {
	// Disabled allows provider to explicitly disable when globally enabled
	Disabled bool `json:"disabled,omitempty"`

	// MaxResults override for this specific provider
	MaxResults *int `json:"max_results,omitempty"`
}

// GetEffectiveConfig returns the effective tool interceptor config for a provider
// Global config is used as base, with provider overrides applied
func (p *Provider) GetEffectiveConfig(global *ToolInterceptorConfig) (*ToolInterceptorConfig, bool) {
	// Provider-level config (preferred)
	if p.ToolInterceptor != nil {
		if p.ToolInterceptorOverride != nil && p.ToolInterceptorOverride.Disabled {
			return nil, false
		}

		base := global
		if base == nil {
			base = &ToolInterceptorConfig{}
		}

		effective := &ToolInterceptorConfig{
			PreferLocalSearch: base.PreferLocalSearch,
			SearchAPI:    base.SearchAPI,
			SearchKey:    base.SearchKey,
			MaxResults:   base.MaxResults,
			ProxyURL:     base.ProxyURL,
			MaxFetchSize: base.MaxFetchSize,
			FetchTimeout: base.FetchTimeout,
			MaxURLLength: base.MaxURLLength,
		}

		if p.ToolInterceptor.PreferLocalSearch {
			effective.PreferLocalSearch = true
		}
		if p.ToolInterceptor.SearchAPI != "" {
			effective.SearchAPI = p.ToolInterceptor.SearchAPI
		}
		if p.ToolInterceptor.SearchKey != "" {
			effective.SearchKey = p.ToolInterceptor.SearchKey
		}
		if p.ToolInterceptor.MaxResults != 0 {
			effective.MaxResults = p.ToolInterceptor.MaxResults
		}
		if p.ToolInterceptor.ProxyURL != "" {
			effective.ProxyURL = p.ToolInterceptor.ProxyURL
		}
		if p.ToolInterceptor.MaxFetchSize != 0 {
			effective.MaxFetchSize = p.ToolInterceptor.MaxFetchSize
		}
		if p.ToolInterceptor.FetchTimeout != 0 {
			effective.FetchTimeout = p.ToolInterceptor.FetchTimeout
		}
		if p.ToolInterceptor.MaxURLLength != 0 {
			effective.MaxURLLength = p.ToolInterceptor.MaxURLLength
		}

		// Apply legacy overrides if present
		if p.ToolInterceptorOverride != nil && p.ToolInterceptorOverride.MaxResults != nil {
			effective.MaxResults = *p.ToolInterceptorOverride.MaxResults
		}

		applyToolInterceptorDefaults(effective)
		return effective, true
	}

	// Legacy override path (requires global enabled)
	if global == nil {
		return nil, false
	}

	// Check if provider has explicitly disabled
	if p.ToolInterceptorOverride != nil && p.ToolInterceptorOverride.Disabled {
		return nil, false
	}

	// Start with global config
	effective := &ToolInterceptorConfig{
		PreferLocalSearch: global.PreferLocalSearch,
		SearchAPI:    global.SearchAPI,
		SearchKey:    global.SearchKey,
		MaxResults:   global.MaxResults,
		ProxyURL:     global.ProxyURL,
		MaxFetchSize: global.MaxFetchSize,
		FetchTimeout: global.FetchTimeout,
		MaxURLLength: global.MaxURLLength,
	}

	// Apply provider overrides
	if p.ToolInterceptorOverride != nil {
		if p.ToolInterceptorOverride.MaxResults != nil {
			effective.MaxResults = *p.ToolInterceptorOverride.MaxResults
		}
	}

	applyToolInterceptorDefaults(effective)

	return effective, true
}

func applyToolInterceptorDefaults(config *ToolInterceptorConfig) {
	if config.MaxResults == 0 {
		config.MaxResults = 10
	}
	if config.MaxFetchSize == 0 {
		config.MaxFetchSize = 1 * 1024 * 1024 // 1MB
	}
	if config.FetchTimeout == 0 {
		config.FetchTimeout = 30 // 30 seconds
	}
	if config.MaxURLLength == 0 {
		config.MaxURLLength = 2000
	}
	// Default to duckduckgo if no search API specified
	if config.SearchAPI == "" {
		config.SearchAPI = "duckduckgo"
	}
}

// IsExpired checks if the OAuth token is expired
func (o *OAuthDetail) IsExpired() bool {
	if o == nil || o.ExpiresAt == "" {
		return false
	}
	// Parse RFC3339 timestamp and check if expired
	expiryTime, err := time.Parse(time.RFC3339, o.ExpiresAt)
	if err != nil {
		return false
	}
	return time.Now().Add(5 * time.Minute).After(expiryTime) // Consider expired if within 5 minutes
}

// Provider represents an AI model api key and provider configuration
type Provider struct {
	UUID          string            `json:"uuid"`
	Name          string            `json:"name"`
	APIBase       string            `json:"api_base"`
	APIStyle      protocol.APIStyle `json:"api_style"` // "openai" or "anthropic", defaults to "openai"
	Token         string            `json:"token"`     // API key for api_key auth type
	NoKeyRequired bool              `json:"no_key_required"`
	Enabled       bool              `json:"enabled"`
	ProxyURL      string            `json:"proxy_url"`              // HTTP or SOCKS proxy URL (e.g., "http://127.0.0.1:7890" or "socks5://127.0.0.1:1080")
	Timeout       int64             `json:"timeout,omitempty"`      // Request timeout in seconds (default: 1800 = 30 minutes)
	Tags          []string          `json:"tags,omitempty"`         // Provider tags for categorization
	Models        []string          `json:"models,omitempty"`       // Available models for this provider (cached)
	LastUpdated   string            `json:"last_updated,omitempty"` // Last update timestamp

	// Auth configuration
	AuthType                AuthType                 `json:"auth_type"`                           // api_key or oauth
	OAuthDetail             *OAuthDetail             `json:"oauth_detail,omitempty"`              // OAuth credentials (only for oauth auth type)
	ToolInterceptor         *ToolInterceptorConfig   `json:"tool_interceptor,omitempty"`          // Provider-level tool interceptor config
	ToolInterceptorOverride *ToolInterceptorOverride `json:"tool_interceptor_override,omitempty"` // Provider-level override for tool interceptor
}

// GetAccessToken returns the access token based on auth type
func (p *Provider) GetAccessToken() string {
	switch p.AuthType {
	case AuthTypeOAuth:
		if p.OAuthDetail != nil {
			return p.OAuthDetail.AccessToken
		}
	case AuthTypeAPIKey, "":
		// Default to api_key for backward compatibility
		return p.Token
	}
	return ""
}

// IsOAuthExpired checks if the OAuth token is expired (only valid for oauth auth type)
func (p *Provider) IsOAuthExpired() bool {
	if p.AuthType == AuthTypeOAuth && p.OAuthDetail != nil {
		return p.OAuthDetail.IsExpired()
	}
	return false
}

// Rule represents a request/response configuration with load balancing support
type Rule struct {
	UUID          string                 `json:"uuid"`
	Scenario      RuleScenario           `json:"scenario,required" yaml:"scenario"` // openai, anthropic, claude_code; defaults to openai
	RequestModel  string                 `json:"request_model" yaml:"request_model"`
	ResponseModel string                 `json:"response_model" yaml:"response_model"`
	Description   string                 `json:"description"`
	Services      []*loadbalance.Service `json:"services" yaml:"services"`
	// CurrentServiceID is persisted to SQLite, not JSON (provider:model format)
	// This identifies the current service for round-robin load balancing
	CurrentServiceID string `json:"-" yaml:"-"`
	// Unified Tactic Configuration
	LBTactic Tactic `json:"lb_tactic" yaml:"lb_tactic"`
	Active   bool   `json:"active" yaml:"active"`
	// Smart Routing Configuration
	SmartEnabled bool                        `json:"smart_enabled" yaml:"smart_enabled"`
	SmartRouting []smartrouting.SmartRouting `json:"smart_routing,omitempty" yaml:"smart_routing,omitempty"`
}

// ToJSON implementation
func (r *Rule) ToJSON() interface{} {
	// Ensure Services is populated
	services := r.GetServices()

	// Create the JSON representation (note: current_service_index is persisted to SQLite, not JSON)
	jsonRule := map[string]interface{}{
		"uuid":           r.UUID,
		"scenario":       r.GetScenario(),
		"request_model":  r.RequestModel,
		"response_model": r.ResponseModel,
		"description":    r.Description,
		"services":       services,
		"lb_tactic":      r.LBTactic,
		"active":         r.Active,
		"smart_enabled":  r.SmartEnabled,
		"smart_routing":  r.SmartRouting,
	}

	return jsonRule
}

// GetServices returns the services to use for this rule
func (r *Rule) GetServices() []*loadbalance.Service {
	if r.Services == nil {
		r.Services = []*loadbalance.Service{}
	}
	return r.Services
}

// GetScenario returns the scenario, defaulting to openai if empty
func (r *Rule) GetScenario() RuleScenario {
	return r.Scenario
}

// GetDefaultProvider returns the provider from the currently selected service using load balancing tactic
func (r *Rule) GetDefaultProvider() string {
	service := r.GetCurrentService()
	if service != nil {
		return service.Provider
	}
	return ""
}

// GetDefaultModel returns the model from the currently selected service using load balancing tactic
func (r *Rule) GetDefaultModel() string {
	service := r.GetCurrentService()
	if service != nil {
		return service.Model
	}
	return ""
}

// GetActiveServices returns all active services with initialized stats
func (r *Rule) GetActiveServices() []*loadbalance.Service {
	var activeServices []*loadbalance.Service
	for i := range r.Services {
		if r.Services[i].Active {
			r.Services[i].InitializeStats()
			activeServices = append(activeServices, r.Services[i])
		}
	}
	return activeServices
}

// GetTacticType returns the load balancing tactic type
func (r *Rule) GetTacticType() loadbalance.TacticType {
	if r.LBTactic.Type != 0 {
		return r.LBTactic.Type
	}
	// Default to round robin
	return loadbalance.TacticRoundRobin
}

// GetUUID returns the rule UUID
func (r *Rule) GetUUID() string {
	return r.UUID
}

// SetCurrentServiceID sets the current service ID (used by RuleStateStore hydration)
func (r *Rule) SetCurrentServiceID(serviceID string) {
	r.CurrentServiceID = serviceID
}

// GetCurrentServiceID returns the current service ID
func (r *Rule) GetCurrentServiceID() string {
	return r.CurrentServiceID
}

// GetCurrentService returns the current active service based on CurrentServiceID
func (r *Rule) GetCurrentService() *loadbalance.Service {
	activeServices := r.GetActiveServices()
	if len(activeServices) == 0 {
		return nil
	}

	// If CurrentServiceID is set, find and return that service
	if r.CurrentServiceID != "" {
		for _, svc := range activeServices {
			if svc.ServiceID() == r.CurrentServiceID && svc.Active {
				return svc
			}
		}
	}

	// Default to first service if CurrentServiceID not found or not set
	return activeServices[0]
}
