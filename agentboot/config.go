package agentboot

import (
	"time"
)

// DefaultConfig returns the default AgentBoot configuration
func DefaultConfig() Config {
	return Config{
		DefaultAgent:            AgentTypeClaude,
		DefaultFormat:           OutputFormatStreamJSON,
		EnableStreamJSON:        true,
		StreamBufferSize:        100,
		DefaultExecutionTimeout: 5 * time.Minute,
	}
}

// DefaultPermissionConfig returns the default permission handler configuration
func DefaultPermissionConfig() PermissionConfig {
	return PermissionConfig{
		DefaultMode:       PermissionModeAuto,
		Timeout:           60 * time.Minute,
		EnableWhitelist:   false,
		RememberDecisions: true,
		DecisionDuration:  24 * time.Hour,
	}
}
