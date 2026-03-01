package mock

import (
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// Config holds the mock agent configuration
type Config struct {
	// MaxIterations is the maximum number of permission cycles (default: 5)
	MaxIterations int `json:"max_iterations"`

	// StepDelay is the delay between steps (default: 1s)
	StepDelay time.Duration `json:"step_delay"`

	// AutoApprove if true, auto-approves permissions without user interaction (default: false)
	AutoApprove bool `json:"auto_approve"`

	// ResponseTemplate is the template for mock responses
	// Supports placeholders: {step}, {total}, {prompt}
	ResponseTemplate string `json:"response_template"`

	// PermissionMode sets how permissions are handled
	PermissionMode agentboot.PermissionMode `json:"permission_mode"`
}

// DefaultConfig returns the default mock agent configuration
func DefaultConfig() Config {
	return Config{
		MaxIterations:    5,
		StepDelay:        1 * time.Second,
		AutoApprove:      false,
		ResponseTemplate: "[Mock Step {step}/{total}] Processing: {prompt}",
		PermissionMode:   agentboot.PermissionModeManual,
	}
}

// Merge merges the given config with defaults
func (c Config) Merge(defaults Config) Config {
	result := defaults
	if c.MaxIterations > 0 {
		result.MaxIterations = c.MaxIterations
	}
	if c.StepDelay > 0 {
		result.StepDelay = c.StepDelay
	}
	if c.ResponseTemplate != "" {
		result.ResponseTemplate = c.ResponseTemplate
	}
	if c.PermissionMode != "" {
		result.PermissionMode = c.PermissionMode
	}
	result.AutoApprove = c.AutoApprove
	return result
}
