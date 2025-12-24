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
}

// ToJSON implementation
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
		"lb_tactic":             r.LBTactic,
		"active":                r.Active,
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

// GetTacticType returns the load balancing tactic type
func (r *Rule) GetTacticType() TacticType {
	if r.LBTactic.Type != 0 {
		return r.LBTactic.Type
	}
	// Default to round robin
	return TacticRoundRobin
}
