package config

import "github.com/tingly-dev/tingly-box/internal/constant"

// RemoteCoderConfig holds configuration for the remote-coder service.
type RemoteCoderConfig struct {
	Port                 int    `json:"port"`
	DBPath               string `json:"db_path"`
	SessionTimeout       string `json:"session_timeout"`
	MessageRetentionDays int    `json:"message_retention_days"`
	RateLimitMax         int    `json:"rate_limit_max"`
	RateLimitWindow      string `json:"rate_limit_window"`
	RateLimitBlock       string `json:"rate_limit_block"`

	// AgentBoot Settings
	DefaultAgent        string `json:"default_agent"`         // "claude"
	DefaultOutputFormat string `json:"default_output_format"` // "text" or "stream-json"
	EnableStreamJSON    bool   `json:"enable_stream_json"`
	StreamBufferSize    int    `json:"stream_buffer_size"`
	ClaudePath          string `json:"claude_path"`
	SkipPermissions     bool   `json:"skip_permissions"`

	// Permission Settings
	PermissionMode    string `json:"permission_mode"`    // "auto", "manual", "skip"
	PermissionTimeout string `json:"permission_timeout"`
	EnableWhitelist   bool   `json:"enable_whitelist"`
	Whitelist         string `json:"whitelist"`  // Comma-separated
	Blacklist         string `json:"blacklist"`  // Comma-separated
	RememberDecisions bool   `json:"remember_decisions"`
	DecisionDuration  string `json:"decision_duration"`
}

func (c *Config) applyRemoteCoderDefaults() bool {
	updated := false
	if c.RemoteCoder.Port == 0 {
		c.RemoteCoder.Port = 18080
		updated = true
	}
	if c.RemoteCoder.DBPath == "" {
		if c.ConfigDir != "" {
			c.RemoteCoder.DBPath = constant.GetDBFile(c.ConfigDir)
			updated = true
		}
	}
	if c.RemoteCoder.SessionTimeout == "" {
		c.RemoteCoder.SessionTimeout = "336h"
		updated = true
	}
	if c.RemoteCoder.MessageRetentionDays == 0 {
		c.RemoteCoder.MessageRetentionDays = 14
		updated = true
	}
	if c.RemoteCoder.RateLimitMax == 0 {
		c.RemoteCoder.RateLimitMax = 5
		updated = true
	}
	if c.RemoteCoder.RateLimitWindow == "" {
		c.RemoteCoder.RateLimitWindow = "5m"
		updated = true
	}
	if c.RemoteCoder.RateLimitBlock == "" {
		c.RemoteCoder.RateLimitBlock = "5m"
		updated = true
	}

	// AgentBoot defaults
	if c.RemoteCoder.DefaultAgent == "" {
		c.RemoteCoder.DefaultAgent = "claude"
		updated = true
	}
	if c.RemoteCoder.DefaultOutputFormat == "" {
		c.RemoteCoder.DefaultOutputFormat = "text"
		updated = true
	}
	if c.RemoteCoder.StreamBufferSize == 0 {
		c.RemoteCoder.StreamBufferSize = 100
		updated = true
	}

	// Permission defaults
	if c.RemoteCoder.PermissionMode == "" {
		c.RemoteCoder.PermissionMode = "auto"
		updated = true
	}
	if c.RemoteCoder.PermissionTimeout == "" {
		c.RemoteCoder.PermissionTimeout = "5m"
		updated = true
	}
	if c.RemoteCoder.DecisionDuration == "" {
		c.RemoteCoder.DecisionDuration = "24h"
		updated = true
	}

	return updated
}
