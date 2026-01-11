package typ

import (
	"time"

	"tingly-box/internal/loadbalance"
	"tingly-box/internal/smart_routing"
)

// APIStyle represents the API style/version for a provider
type APIStyle string

const (
	APIStyleOpenAI    APIStyle = "openai"
	APIStyleAnthropic APIStyle = "anthropic"
	APIStyleGoogle    APIStyle = "google"
)

// RuleScenario represents the scenario for a routing rule
type RuleScenario string

const (
	ScenarioOpenAI     RuleScenario = "openai"
	ScenarioAnthropic  RuleScenario = "anthropic"
	ScenarioClaudeCode RuleScenario = "claude_code"
)

// ScenarioFlags represents configuration flags for a scenario
type ScenarioFlags struct {
	Unified  bool `json:"unified" yaml:"unified"`   // Single configuration for all models
	Separate bool `json:"separate" yaml:"separate"` // Separate configuration for each model
	Smart    bool `json:"smart" yaml:"smart"`       // Smart mode with automatic optimization
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
	UUID          string   `json:"uuid"`
	Name          string   `json:"name"`
	APIBase       string   `json:"api_base"`
	APIStyle      APIStyle `json:"api_style"` // "openai" or "anthropic", defaults to "openai"
	Token         string   `json:"token"`     // API key for api_key auth type
	NoKeyRequired bool     `json:"no_key_required"`
	Enabled       bool     `json:"enabled"`
	ProxyURL      string   `json:"proxy_url"`              // HTTP or SOCKS proxy URL (e.g., "http://127.0.0.1:7890" or "socks5://127.0.0.1:1080")
	Timeout       int64    `json:"timeout,omitempty"`      // Request timeout in seconds (default: 1800 = 30 minutes)
	Tags          []string `json:"tags,omitempty"`         // Provider tags for categorization
	Models        []string `json:"models,omitempty"`       // Available models for this provider (cached)
	LastUpdated   string   `json:"last_updated,omitempty"` // Last update timestamp

	// Auth configuration
	AuthType    AuthType     `json:"auth_type"`              // api_key or oauth
	OAuthDetail *OAuthDetail `json:"oauth_detail,omitempty"` // OAuth credentials (only for oauth auth type)
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
	UUID                string                `json:"uuid"`
	Scenario            RuleScenario          `json:"scenario,required" yaml:"scenario"` // openai, anthropic, claude_code; defaults to openai
	RequestModel        string                `json:"request_model" yaml:"request_model"`
	ResponseModel       string                `json:"response_model" yaml:"response_model"`
	Description         string                `json:"description"`
	Services            []loadbalance.Service `json:"services" yaml:"services"`
	CurrentServiceIndex int                   `json:"current_service_index" yaml:"current_service_index"`
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

	// Create the JSON representation
	jsonRule := map[string]interface{}{
		"uuid":                  r.UUID,
		"scenario":              r.GetScenario(),
		"request_model":         r.RequestModel,
		"response_model":        r.ResponseModel,
		"description":           r.Description,
		"services":              services,
		"current_service_index": r.CurrentServiceIndex,
		"lb_tactic":             r.LBTactic,
		"active":                r.Active,
		"smart_enabled":         r.SmartEnabled,
		"smart_routing":         r.SmartRouting,
	}

	return jsonRule
}

// GetServices returns the services to use for this rule
func (r *Rule) GetServices() []loadbalance.Service {
	if r.Services == nil {
		r.Services = []loadbalance.Service{}
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
			activeServices = append(activeServices, &r.Services[i])
		}
	}
	return activeServices
}

// GetCurrentService returns the current active service based on CurrentServiceIndex
func (r *Rule) GetCurrentService() *loadbalance.Service {
	activeServices := r.GetActiveServices()
	if len(activeServices) == 0 {
		return nil
	}

	currentIndex := r.CurrentServiceIndex % len(activeServices)
	return activeServices[currentIndex]
}

// GetTacticType returns the load balancing tactic type
func (r *Rule) GetTacticType() loadbalance.TacticType {
	if r.LBTactic.Type != 0 {
		return r.LBTactic.Type
	}
	// Default to round robin
	return loadbalance.TacticRoundRobin
}
