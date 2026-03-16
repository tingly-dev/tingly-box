package guardrails

// Config is the top-level guardrails configuration.
type Config struct {
	Version       string          `json:"version,omitempty" yaml:"version,omitempty"`
	Strategy      CombineStrategy `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	ErrorStrategy ErrorStrategy   `json:"error_strategy,omitempty" yaml:"error_strategy,omitempty"`
	Rules         []RuleConfig    `json:"rules" yaml:"rules"`
}

// RuleConfig defines a single rule with flexible parameters.
type RuleConfig struct {
	ID      string                 `json:"id" yaml:"id"`
	Name    string                 `json:"name" yaml:"name"`
	Type    RuleType               `json:"type" yaml:"type"`
	Enabled bool                   `json:"enabled" yaml:"enabled"`
	Scope   Scope                  `json:"scope,omitempty" yaml:"scope,omitempty"`
	Params  map[string]interface{} `json:"params,omitempty" yaml:"params,omitempty"`
}
