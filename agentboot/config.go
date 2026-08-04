package agentboot

import "time"

// Config holds the AgentService configuration.
type Config struct {
	DefaultAgent            AgentType     `json:"default_agent"`
	DefaultFormat           OutputFormat  `json:"default_format"`
	StreamBufferSize        int           `json:"stream_buffer_size"`
	DefaultExecutionTimeout time.Duration `json:"default_execution_timeout"`
}

// DefaultConfig returns the default AgentService configuration
func DefaultConfig() Config {
	return Config{
		DefaultAgent:            AgentTypeClaude,
		DefaultFormat:           OutputFormatStreamJSON,
		StreamBufferSize:        100,
		DefaultExecutionTimeout: 0, // no timeout
	}
}
