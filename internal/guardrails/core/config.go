package core

// Config is the top-level guardrails configuration.
type Config struct {
	Strategy      CombineStrategy `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	ErrorStrategy ErrorStrategy   `json:"error_strategy,omitempty" yaml:"error_strategy,omitempty"`
	Groups        []PolicyGroup   `json:"groups,omitempty" yaml:"groups,omitempty"`
	Policies      []Policy        `json:"policies,omitempty" yaml:"policies,omitempty"`
}
