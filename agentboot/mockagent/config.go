package mock

import (
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// Config holds the mock agent configuration.
//
// Two execution modes:
//
//   - If Script is non-empty, the agent plays it step by step.
//   - Otherwise, the agent generates a linear default script from
//     MaxIterations / AskUserQuestionFrequency / ResponseTemplate.
type Config struct {
	// Script declares an explicit event sequence. When set, MaxIterations,
	// AskUserQuestionFrequency and ResponseTemplate are ignored.
	Script []Step `json:"-"`

	// MaxIterations is the legacy linear-default iteration count.
	MaxIterations int `json:"max_iterations"`

	// StepDelay is the inter-step delay (default: 1s).
	StepDelay time.Duration `json:"step_delay"`

	// AutoApprove short-circuits PermissionStep / AskStep without invoking
	// the handler.
	AutoApprove bool `json:"auto_approve"`

	// ResponseTemplate is the template for legacy linear-default assistant
	// messages. Supports placeholders: {step}, {total}, {prompt}.
	ResponseTemplate string `json:"response_template"`

	// PermissionMode is informational; the scriptable engine routes through
	// the handler regardless.
	PermissionMode agentboot.PermissionMode `json:"permission_mode"`

	// AskUserQuestionFrequency configures the linear-default script to emit
	// an AskUserQuestion every N steps (0 = never).
	AskUserQuestionFrequency int `json:"ask_user_question_frequency"`
}

// DefaultConfig returns the default mock agent configuration.
func DefaultConfig() Config {
	return Config{
		MaxIterations:    5,
		StepDelay:        0,
		AutoApprove:      false,
		ResponseTemplate: "[Mock Step {step}/{total}] Processing: {prompt}",
		PermissionMode:   agentboot.PermissionModeManual,
	}
}

// Merge merges the given config with defaults.
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
	if c.AskUserQuestionFrequency > 0 {
		result.AskUserQuestionFrequency = c.AskUserQuestionFrequency
	}
	if len(c.Script) > 0 {
		result.Script = c.Script
	}
	result.AutoApprove = c.AutoApprove
	return result
}
