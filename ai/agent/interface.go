package agent

// AgentConfig defines the interface for agent configuration operations.
// Each agent type implements this interface independently.
type AgentConfig interface {
	// Apply applies the agent configuration files.
	// Does NOT handle routing rules - that's handled separately.
	Apply(params interface{}) (*ApplyAgentResult, error)

	// Restore restores configuration files from backup.
	Restore() (*RestoreAgentResult, error)
}

// AgentConfigInfo provides metadata about an agent config implementation.
type AgentConfigInfo struct {
	// Type is the agent type
	Type AgentType
	// Name is the display name
	Name string
	// Description is a brief description
	Description string
	// ConfigFiles lists the configuration files this agent uses
	ConfigFiles []string
	// Scenario is the corresponding routing rule scenario
	Scenario string
}

// Registry holds agent config implementations
type Registry struct {
	configs map[AgentType]AgentConfig
}

// NewRegistry creates a new agent config registry
func NewRegistry() *Registry {
	return &Registry{
		configs: make(map[AgentType]AgentConfig),
	}
}

// Register registers an agent config implementation
func (r *Registry) Register(agentType AgentType, config AgentConfig) {
	r.configs[agentType] = config
}

// Get returns the agent config for the given type
func (r *Registry) Get(agentType AgentType) (AgentConfig, bool) {
	config, ok := r.configs[agentType]
	return config, ok
}

// DefaultRegistry is the global registry with all built-in agents
var DefaultRegistry = NewRegistry()

func init() {
	// Register all built-in agents
	DefaultRegistry.Register(AgentTypeClaudeCode, &ClaudeCodeConfig{})
	DefaultRegistry.Register(AgentTypeOpenCode, &OpenCodeConfig{})
	DefaultRegistry.Register(AgentTypeCodex, &CodexConfig{})
}
