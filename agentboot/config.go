package agentboot

import "time"

// RetryConfig bounds the automatic retry of pre-output turn failures — a
// launch failure, an early process crash, or a stream that ended without a
// result before the agent produced anything (see [PreOutputFailure]). Only
// failures where no output was seen are retried, so a retry can never
// re-run tools that already executed.
type RetryConfig struct {
	// MaxRetries is the number of attempts beyond the first. 0 disables retry.
	MaxRetries int `json:"max_retries"`
	// BaseBackoff is the delay before the first retry; it doubles per attempt.
	BaseBackoff time.Duration `json:"base_backoff"`
	// MaxBackoff caps the per-attempt delay.
	MaxBackoff time.Duration `json:"max_backoff"`
}

// DefaultRetryConfig returns the default retry settings: two retries with a
// short capped backoff — pre-output failures are launch flakes and startup
// crashes, not sustained outages, so retrying should not stall a turn long.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:  2,
		BaseBackoff: 250 * time.Millisecond,
		MaxBackoff:  2 * time.Second,
	}
}

// backoff returns the delay before retry attempt n (0-based).
func (c RetryConfig) backoff(n int) time.Duration {
	base := c.BaseBackoff
	if base <= 0 {
		base = DefaultRetryConfig().BaseBackoff
	}
	max := c.MaxBackoff
	if max <= 0 {
		max = DefaultRetryConfig().MaxBackoff
	}
	d := base
	for i := 0; i < n && d < max; i++ {
		d *= 2
	}
	if d > max {
		d = max
	}
	return d
}

// Config holds the AgentService configuration.
type Config struct {
	DefaultAgent            AgentType     `json:"default_agent"`
	DefaultFormat           OutputFormat  `json:"default_format"`
	StreamBufferSize        int           `json:"stream_buffer_size"`
	DefaultExecutionTimeout time.Duration `json:"default_execution_timeout"`
	Retry                   RetryConfig   `json:"retry,omitempty"`
}

// DefaultConfig returns the default AgentService configuration
func DefaultConfig() Config {
	return Config{
		DefaultAgent:            AgentTypeClaude,
		DefaultFormat:           OutputFormatStreamJSON,
		StreamBufferSize:        100,
		DefaultExecutionTimeout: 0, // no timeout
		Retry:                   DefaultRetryConfig(),
	}
}
