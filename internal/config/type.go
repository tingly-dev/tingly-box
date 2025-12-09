package config

// APIStyle represents the API style/version for a provider
type APIStyle string

const (
	APIStyleOpenAI    APIStyle = "openai"
	APIStyleAnthropic APIStyle = "anthropic"
)

// Provider represents an AI model provider configuration
type Provider struct {
	Name     string   `json:"name"`
	APIBase  string   `json:"api_base"`
	APIStyle APIStyle `json:"api_style"` // "openai" or "anthropic", defaults to "openai"
	Token    string   `json:"token"`
	Enabled  bool     `json:"enabled"`
}

// Rule represents a request/response configuration with load balancing support
type Rule struct {
	RequestModel        string    `yaml:"request_model" json:"request_model"`                 // The "tingly" value
	ResponseModel       string    `yaml:"response_model" json:"response_model"`               // Response model configuration
	Services            []Service `yaml:"services" json:"services"`                           // Multiple services for load balancing
	CurrentServiceIndex int       `yaml:"current_service_index" json:"current_service_index"` // Currently active service index
	Tactic              string    `yaml:"tactic" json:"tactic"`                               // Load balancing strategy (round_robin, token_based, hybrid)
	Active              bool      `yaml:"active" json:"active"`                               // Whether this rule is active (default: true)
}

// GetServices returns the services to use for this rule
func (r *Rule) GetServices() []Service {
	return r.Services
}

// GetDefaultProvider returns the provider from the currently selected service using load balancing tactic
func (r *Rule) GetDefaultProvider() string {
	service := r.GetSelectedService()
	if service != nil {
		return service.Provider
	}
	return ""
}

// GetDefaultModel returns the model from the currently selected service using load balancing tactic
func (r *Rule) GetDefaultModel() string {
	service := r.GetSelectedService()
	if service != nil {
		return service.Model
	}
	return ""
}

// GetSelectedService returns the currently selected service using the load balancing tactic
func (r *Rule) GetSelectedService() *Service {
	if len(r.Services) == 0 {
		return nil
	}

	// Filter active services and initialize stats
	var activeServices []*Service
	for i := range r.Services {
		if r.Services[i].Active {
			r.Services[i].InitializeStats()
			activeServices = append(activeServices, &r.Services[i])
		}
	}

	if len(activeServices) == 0 {
		return nil
	}

	// For single service rules, return it directly
	if len(activeServices) == 1 {
		return activeServices[0]
	}

	// Use the configured tactic to select service
	tacticType := r.GetTacticType()
	tactic := CreateTactic(tacticType, nil)
	if tactic != nil {
		return tactic.SelectService(r)
	}

	// Fallback to first active service
	return activeServices[0]
}

// GetTacticType returns the load balancing tactic type
func (r *Rule) GetTacticType() TacticType {
	if r.Tactic == "" {
		// Default to round robin
		return TacticRoundRobin
	}
	return ParseTacticType(r.Tactic)
}
