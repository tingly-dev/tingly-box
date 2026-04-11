package core

import (
	"context"

	"github.com/gin-gonic/gin"
)

// Verdict is the overall decision from a policy or engine.
type Verdict string

const (
	VerdictAllow  Verdict = "allow"
	VerdictReview Verdict = "review"
	VerdictMask   Verdict = "mask"
	VerdictRedact Verdict = "redact"
	VerdictBlock  Verdict = "block"
)

// CombineStrategy controls how multiple policy verdicts are merged.
type CombineStrategy string

const (
	StrategyMostSevere CombineStrategy = "most_severe"
	StrategyBlockOnAny CombineStrategy = "block_on_any"
)

// ErrorStrategy controls the fallback verdict when a policy evaluation fails.
type ErrorStrategy string

const (
	ErrorStrategyAllow  ErrorStrategy = "allow"
	ErrorStrategyReview ErrorStrategy = "review"
	ErrorStrategyBlock  ErrorStrategy = "block"
)

// Direction indicates whether the input is a request or response.
type Direction string

const (
	DirectionRequest  Direction = "request"
	DirectionResponse Direction = "response"
)

// ContentType identifies a portion of Content.
type ContentType string

const (
	ContentTypeText     ContentType = "text"
	ContentTypeMessages ContentType = "messages"
	ContentTypeCommand  ContentType = "command"
)

// Payload carries the protocol-level raw request/response objects associated
// with one guardrails processing run. These fields are runtime-only and are not
// part of the persisted policy input schema.
type Payload struct {
	Protocol string `json:"-" yaml:"-"`
	Request  any    `json:"-" yaml:"-"`
	Response any    `json:"-" yaml:"-"`
}

// InputRuntime stores request-scoped runtime integrations for one processing
// run. These hooks are optional and should not be used by policy evaluators.
type InputRuntime struct {
	Context *gin.Context `json:"-" yaml:"-"`
}

// InputState stores runtime-only state associated with one guardrails
// processing run. Policy evaluators should rely on the semantic fields on Input
// and not depend on these mutable runtime details.
type InputState struct {
	CredentialMask *CredentialMaskState   `json:"-" yaml:"-"`
	Evaluation     *Result                `json:"-" yaml:"-"`
	Values         map[string]interface{} `json:"-" yaml:"-"`
}

// Input is the unified guardrails processing context.
//
// The top section contains the stable semantic fields consumed by policy
// evaluation. Request adapters may also place request-focused extracted text
// into Content.Text, while keeping the full conversation history in
// Content.Messages.
//
// The middle section stores lightweight extraction hints derived from the raw
// protocol payload. These fields avoid reparsing the same request structure in
// later pipeline stages.
//
// The bottom section carries runtime-only attachments and should not be relied
// on by policy implementations.
type Input struct {
	Scenario  string                 `json:"scenario,omitempty" yaml:"scenario,omitempty"`
	Model     string                 `json:"model,omitempty" yaml:"model,omitempty"`
	Direction Direction              `json:"direction" yaml:"direction"`
	Tags      []string               `json:"tags,omitempty" yaml:"tags,omitempty"`
	Content   Content                `json:"content" yaml:"content"`
	Metadata  map[string]interface{} `json:"metadata,omitempty" yaml:"metadata,omitempty"`

	HasToolResult        bool `json:"has_tool_result,omitempty" yaml:"has_tool_result,omitempty"`
	ToolResultBlockCount int  `json:"tool_result_block_count,omitempty" yaml:"tool_result_block_count,omitempty"`
	ToolResultPartCount  int  `json:"tool_result_part_count,omitempty" yaml:"tool_result_part_count,omitempty"`

	Payload Payload      `json:"-" yaml:"-"`
	State   InputState   `json:"-" yaml:"-"`
	Runtime InputRuntime `json:"-" yaml:"-"`
}

// Text returns the combined text for guardrails matching.
func (i Input) Text() string {
	return i.Content.CombinedText()
}

// ProviderName returns the provider stored in metadata when present.
func (i Input) ProviderName() string {
	if i.Metadata == nil {
		return ""
	}
	if provider, ok := i.Metadata["provider"].(string); ok {
		return provider
	}
	return ""
}

// RequestModel returns the upstream request model stored in metadata when
// present.
func (i Input) RequestModel() string {
	if i.Metadata == nil {
		return ""
	}
	if model, ok := i.Metadata["request_model"].(string); ok {
		return model
	}
	return ""
}

// SetContextValue publishes a request-scoped runtime value when an optional
// gin context is attached to the input.
func (i Input) SetContextValue(key string, value any) {
	if i.Runtime.Context == nil {
		return
	}
	i.Runtime.Context.Set(key, value)
}

// CredentialMaskState returns the request-scoped credential masking state when
// available via runtime hooks, falling back to the input state snapshot.
func (i Input) CredentialMaskState() *CredentialMaskState {
	if i.Runtime.Context != nil {
		if existing, ok := i.Runtime.Context.Get(CredentialMaskStateContextKey); ok {
			if state, ok := existing.(*CredentialMaskState); ok && state != nil {
				return state
			}
		}
	}
	return i.State.CredentialMask
}

// PolicyType identifies a policy evaluator implementation.
type PolicyType string

// PolicyResult captures a single policy decision.
type PolicyResult struct {
	PolicyID   string                 `json:"policy_id" yaml:"policy_id"`
	PolicyName string                 `json:"policy_name" yaml:"policy_name"`
	PolicyType PolicyType             `json:"policy_type" yaml:"policy_type"`
	Verdict    Verdict                `json:"verdict" yaml:"verdict"`
	Reason     string                 `json:"reason,omitempty" yaml:"reason,omitempty"`
	Evidence   map[string]interface{} `json:"evidence,omitempty" yaml:"evidence,omitempty"`
}

// PolicyError captures an evaluation failure for a policy.
type PolicyError struct {
	PolicyID   string     `json:"policy_id" yaml:"policy_id"`
	PolicyName string     `json:"policy_name" yaml:"policy_name"`
	PolicyType PolicyType `json:"policy_type" yaml:"policy_type"`
	Error      string     `json:"error" yaml:"error"`
}

// Result is the aggregated guardrails decision.
type Result struct {
	Verdict Verdict        `json:"verdict" yaml:"verdict"`
	Reasons []PolicyResult `json:"reasons,omitempty" yaml:"reasons,omitempty"`
	Errors  []PolicyError  `json:"errors,omitempty" yaml:"errors,omitempty"`
}

// Evaluator evaluates a single guardrail policy.
type Evaluator interface {
	ID() string
	Name() string
	Type() PolicyType
	Enabled() bool
	Evaluate(ctx context.Context, input Input) (PolicyResult, error)
}
