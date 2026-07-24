package agentboot

// DefaultConfig returns the default AgentBoot configuration
func DefaultConfig() Config {
	return Config{
		DefaultAgent:            AgentTypeClaude,
		DefaultFormat:           OutputFormatStreamJSON,
		EnableStreamJSON:        true,
		StreamBufferSize:        100,
		DefaultExecutionTimeout: 0, // no timeout
	}
}
