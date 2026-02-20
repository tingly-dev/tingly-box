package agentboot

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// LoadConfigFromEnv loads configuration from environment variables
func LoadConfigFromEnv() Config {
	config := Config{
		DefaultAgent:      AgentTypeClaude,
		DefaultFormat:     OutputFormatText,
		EnableStreamJSON:  true,
		StreamBufferSize:  100,
	}

	// Load default agent
	if agent := os.Getenv("AGENTBOOT_DEFAULT_AGENT"); agent != "" {
		config.DefaultAgent = AgentType(agent)
	}

	// Load default format
	if format := os.Getenv("AGENTBOOT_DEFAULT_FORMAT"); format != "" {
		config.DefaultFormat = OutputFormat(format)
	}

	// Load stream-json enable
	if enable := os.Getenv("AGENTBOOT_ENABLE_STREAM_JSON"); enable != "" {
		config.EnableStreamJSON = enable == "true" || enable == "1"
	}

	// Load buffer size
	if bufSize := os.Getenv("AGENTBOOT_STREAM_BUFFER_SIZE"); bufSize != "" {
		var size int
		if _, err := fmt.Sscanf(bufSize, "%d", &size); err == nil && size > 0 {
			config.StreamBufferSize = size
		}
	}

	return config
}

// ParsePermissionConfig parses permission-related configuration from environment
func ParsePermissionConfig() PermissionConfig {
	config := PermissionConfig{
		DefaultMode:       PermissionModeAuto,
		Timeout:           5 * time.Minute,
		EnableWhitelist:   false,
		RememberDecisions: true,
		DecisionDuration:  24 * time.Hour,
	}

	// Load permission mode
	if mode := os.Getenv("RCC_PERMISSION_MODE"); mode != "" {
		if parsedMode, ok := ParsePermissionMode(mode); ok {
			config.DefaultMode = parsedMode
		}
	}

	// Load timeout
	if timeout := os.Getenv("RCC_PERMISSION_TIMEOUT"); timeout != "" {
		if duration, err := time.ParseDuration(timeout); err == nil {
			config.Timeout = duration
		}
	}

	// Load whitelist
	config.EnableWhitelist = os.Getenv("RCC_ENABLE_WHITELIST") == "true" || os.Getenv("RCC_ENABLE_WHITELIST") == "1"
	if whitelist := os.Getenv("RCC_WHITELIST"); whitelist != "" {
		config.Whitelist = strings.Split(whitelist, ",")
		for i, tool := range config.Whitelist {
			config.Whitelist[i] = strings.TrimSpace(tool)
		}
	}

	// Load blacklist
	if blacklist := os.Getenv("RCC_BLACKLIST"); blacklist != "" {
		config.Blacklist = strings.Split(blacklist, ",")
		for i, tool := range config.Blacklist {
			config.Blacklist[i] = strings.TrimSpace(tool)
		}
	}

	// Load remember decisions
	if remember := os.Getenv("RCC_REMEMBER_DECISIONS"); remember != "" {
		config.RememberDecisions = remember == "true" || remember == "1"
	}

	// Load decision duration
	if duration := os.Getenv("RCC_DECISION_DURATION"); duration != "" {
		if d, err := time.ParseDuration(duration); err == nil {
			config.DecisionDuration = d
		}
	}

	return config
}

// PermissionConfig holds permission handler configuration
type PermissionConfig struct {
	DefaultMode       PermissionMode `json:"default_mode"`
	Timeout           time.Duration  `json:"timeout"`
	EnableWhitelist   bool            `json:"enable_whitelist"`
	Whitelist         []string        `json:"whitelist"`
	Blacklist         []string        `json:"blacklist"`
	RememberDecisions bool            `json:"remember_decisions"`
	DecisionDuration  time.Duration  `json:"decision_duration"`
}
