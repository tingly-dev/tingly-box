package agentboot

import (
	"fmt"
	"sync"
)

// Config holds the AgentBoot configuration
type Config struct {
	DefaultAgent      AgentType     `json:"default_agent"`
	DefaultFormat     OutputFormat  `json:"default_format"`
	EnableStreamJSON  bool          `json:"enable_stream_json"`
	StreamBufferSize  int           `json:"stream_buffer_size"`
}

// AgentBoot manages agent instances
type AgentBoot struct {
	mu     sync.RWMutex
	config Config
	agents map[AgentType]Agent
}

// New creates a new AgentBoot instance
func New(config Config) *AgentBoot {
	if config.DefaultAgent == "" {
		config.DefaultAgent = AgentTypeClaude
	}
	if config.DefaultFormat == "" {
		config.DefaultFormat = OutputFormatText
	}
	if config.StreamBufferSize == 0 {
		config.StreamBufferSize = 100
	}

	return &AgentBoot{
		config: config,
		agents: make(map[AgentType]Agent),
	}
}

// RegisterAgent registers a new agent type
func (ab *AgentBoot) RegisterAgent(agentType AgentType, agent Agent) {
	ab.mu.Lock()
	defer ab.mu.Unlock()
	ab.agents[agentType] = agent
}

// GetAgent returns an agent by type
func (ab *AgentBoot) GetAgent(agentType AgentType) (Agent, error) {
	ab.mu.RLock()
	defer ab.mu.RUnlock()

	agent, exists := ab.agents[agentType]
	if !exists {
		return nil, fmt.Errorf("agent type not supported: %s", agentType)
	}
	return agent, nil
}

// MustGetAgent returns an agent by type or panics
func (ab *AgentBoot) MustGetAgent(agentType AgentType) Agent {
	agent, err := ab.GetAgent(agentType)
	if err != nil {
		panic(err)
	}
	return agent
}

// GetDefaultAgent returns the default agent
func (ab *AgentBoot) GetDefaultAgent() (Agent, error) {
	return ab.GetAgent(ab.config.DefaultAgent)
}

// SetDefaultAgent sets the default agent type
func (ab *AgentBoot) SetDefaultAgent(agentType AgentType) error {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	if _, exists := ab.agents[agentType]; !exists {
		return fmt.Errorf("agent type not registered: %s", agentType)
	}

	ab.config.DefaultAgent = agentType
	return nil
}

// GetConfig returns the current configuration
func (ab *AgentBoot) GetConfig() Config {
	ab.mu.RLock()
	defer ab.mu.RUnlock()
	return ab.config
}

// ListAgents returns all registered agent types
func (ab *AgentBoot) ListAgents() []AgentType {
	ab.mu.RLock()
	defer ab.mu.RUnlock()

	types := make([]AgentType, 0, len(ab.agents))
	for agentType := range ab.agents {
		types = append(types, agentType)
	}
	return types
}
