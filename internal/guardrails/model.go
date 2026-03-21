package guardrails

import "context"

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

// Input is the normalized data sent to guardrails.
type Input struct {
	Scenario  string                 `json:"scenario,omitempty" yaml:"scenario,omitempty"`
	Model     string                 `json:"model,omitempty" yaml:"model,omitempty"`
	Direction Direction              `json:"direction" yaml:"direction"`
	Tags      []string               `json:"tags,omitempty" yaml:"tags,omitempty"`
	Content   Content                `json:"content" yaml:"content"`
	Metadata  map[string]interface{} `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// Text returns the combined text for guardrails matching.
func (i Input) Text() string {
	return i.Content.CombinedText()
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

// Guardrails is the interface for evaluating input.
type Guardrails interface {
	Evaluate(ctx context.Context, input Input) (Result, error)
}
