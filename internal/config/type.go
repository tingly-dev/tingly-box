package config

import "time"

// APIStyle represents the API style/version for a provider
type APIStyle string

const (
	APIStyleOpenAI    APIStyle = "openai"
	APIStyleAnthropic APIStyle = "anthropic"
)

// Provider represents an AI model api key and provider configuration
type Provider struct {
	UUID        string        `json:"uuid"`
	Name        string        `json:"name"`
	APIBase     string        `json:"api_base"`
	APIStyle    APIStyle      `json:"api_style"` // "openai" or "anthropic", defaults to "openai"
	Token       string        `json:"token"`
	Enabled     bool          `json:"enabled"`
	ProxyURL    string        `json:"proxy_url"`              // HTTP or SOCKS proxy URL (e.g., "http://127.0.0.1:7890" or "socks5://127.0.0.1:1080")
	Timeout     time.Duration `json:"timeout,omitempty"`      // Request timeout in seconds (default: 30,000)
	Tags        []string      `json:"tags,omitempty"`         // Provider tags for categorization
	Models      []string      `json:"models,omitempty"`       // Available models for this provider (cached)
	LastUpdated string        `json:"last_updated,omitempty"` // Last update timestamp
}

// Rule represents a request/response configuration with load balancing support
type Rule struct {
	UUID                string    `json:"uuid"`
	RequestModel        string    `json:"request_model" yaml:"request_model"`
	ResponseModel       string    `json:"response_model" yaml:"response_model"`
	Services            []Service `json:"services" yaml:"services"`
	CurrentServiceIndex int       `json:"current_service_index" yaml:"current_service_index"`
	// Unified Tactic Configuration
	LBTactic Tactic `json:"lb_tactic" yaml:"lb_tactic"`
	Active   bool   `json:"active" yaml:"active"`
	// Deprecated fields kept for Unmarshal migration logic only
	// Deprecated
	Tactic string `yaml:"tactic" json:"tactic"` // Load balancing strategy (round_robin, token_based, hybrid)
	// Deprecated
	TacticParams map[string]interface{} `yaml:"tactic_params" json:"tactic_params,omitempty"`
}

// ToJSON implementation with backward compatibility
func (r *Rule) ToJSON() interface{} {
	// Ensure Services is populated
	services := r.GetServices()

	// Create the JSON representation
	jsonRule := map[string]interface{}{
		"uuid":                  r.UUID,
		"request_model":         r.RequestModel,
		"response_model":        r.ResponseModel,
		"services":              services,
		"current_service_index": r.CurrentServiceIndex,
		"active":                r.Active,
	}

	// Use lb_tactic if it's configured (check if params are not nil or if it's not the default empty tactic)
	if r.LBTactic.Params != nil {
		jsonRule["lb_tactic"] = r.LBTactic
	} else {
		// Fall back to deprecated fields for backward compatibility
		if r.Tactic != "" {
			jsonRule["tactic"] = r.Tactic
			jsonRule["tactic_params"] = r.TacticParams
		} else {
			// Default tactic if none is set
			jsonRule["tactic"] = "round_robin"
			jsonRule["tactic_params"] = map[string]interface{}{
				"request_threshold": 100,
			}
		}
	}

	return jsonRule
}

// GetServices returns the services to use for this rule
func (r *Rule) GetServices() []Service {
	if r.Services == nil {
		r.Services = []Service{}
	}
	return r.Services
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
func (r *Rule) GetActiveServices() []*Service {
	var activeServices []*Service
	for i := range r.Services {
		if r.Services[i].Active {
			r.Services[i].InitializeStats()
			activeServices = append(activeServices, &r.Services[i])
		}
	}
	return activeServices
}

// GetCurrentService returns the current active service based on CurrentServiceIndex
func (r *Rule) GetCurrentService() *Service {
	activeServices := r.GetActiveServices()
	if len(activeServices) == 0 {
		return nil
	}

	currentIndex := r.CurrentServiceIndex % len(activeServices)
	return activeServices[currentIndex]
}

// convertLegacyParams converts legacy map[string]interface{} to proper TacticParams
func convertLegacyParams(tacticStr string, legacyParams map[string]interface{}) TacticParams {
	tacticType := ParseTacticType(tacticStr)

	switch tacticType {
	case TacticRoundRobin:
		if rt, ok := legacyParams["request_threshold"].(int64); ok {
			return &RoundRobinParams{RequestThreshold: rt}
		} else if rt, ok := legacyParams["request_threshold"].(float64); ok {
			return &RoundRobinParams{RequestThreshold: int64(rt)}
		}
		return &RoundRobinParams{RequestThreshold: 100}

	case TacticTokenBased:
		if tt, ok := legacyParams["token_threshold"].(int64); ok {
			return &TokenBasedParams{TokenThreshold: tt}
		} else if tt, ok := legacyParams["token_threshold"].(float64); ok {
			return &TokenBasedParams{TokenThreshold: int64(tt)}
		}
		return &TokenBasedParams{TokenThreshold: 10000}

	case TacticHybrid:
		requestThreshold := int64(100)
		tokenThreshold := int64(10000)

		if rt, ok := legacyParams["request_threshold"].(int64); ok {
			requestThreshold = rt
		} else if rt, ok := legacyParams["request_threshold"].(float64); ok {
			requestThreshold = int64(rt)
		}

		if tt, ok := legacyParams["token_threshold"].(int64); ok {
			tokenThreshold = tt
		} else if tt, ok := legacyParams["token_threshold"].(float64); ok {
			tokenThreshold = int64(tt)
		}

		return &HybridParams{
			RequestThreshold: requestThreshold,
			TokenThreshold:   tokenThreshold,
		}

	case TacticRandom:
		return &RandomParams{}

	default:
		return &RoundRobinParams{RequestThreshold: 100}
	}
}

// GetTacticType returns the load balancing tactic type
func (r *Rule) GetTacticType() TacticType {
	// Check new LBTactic field first
	if r.LBTactic.Type != 0 {
		return r.LBTactic.Type
	}

	// Fall back to deprecated Tactic field
	if r.Tactic != "" {
		return ParseTacticType(r.Tactic)
	}

	// Default to round robin
	return TacticRoundRobin
}
