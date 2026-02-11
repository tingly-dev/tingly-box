package config

import "path/filepath"

// RemoteCoderConfig holds configuration for the remote-coder service.
type RemoteCoderConfig struct {
	Port                 int    `json:"port"`
	DBPath               string `json:"db_path"`
	SessionTimeout       string `json:"session_timeout"`
	MessageRetentionDays int    `json:"message_retention_days"`
	RateLimitMax         int    `json:"rate_limit_max"`
	RateLimitWindow      string `json:"rate_limit_window"`
	RateLimitBlock       string `json:"rate_limit_block"`
}

func (c *Config) applyRemoteCoderDefaults() bool {
	updated := false
	if c.RemoteCoder.Port == 0 {
		c.RemoteCoder.Port = 18080
		updated = true
	}
	if c.RemoteCoder.DBPath == "" {
		if c.ConfigDir != "" {
			c.RemoteCoder.DBPath = filepath.Join(c.ConfigDir, "tingly-remote-coder.db")
			updated = true
		}
	}
	if c.RemoteCoder.SessionTimeout == "" {
		c.RemoteCoder.SessionTimeout = "30m"
		updated = true
	}
	if c.RemoteCoder.MessageRetentionDays == 0 {
		c.RemoteCoder.MessageRetentionDays = 7
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
	return updated
}
