package config

// Rule represents a request/response configuration with provider and default model
type Rule struct {
	RequestModel  string `yaml:"request_model" json:"request_model"`   // The "tingly" value
	ResponseModel string `yaml:"response_model" json:"response_model"` // Response model configuration
	Provider      string `yaml:"provider" json:"provider"`             // Provider for this request config
	DefaultModel  string `yaml:"default_model" json:"default_model"`   // Default model for the provider
	Active        bool   `yaml:"active" json:"active"`                 // Whether this rule is active (default: true)
}
