package guardrails

import "context"

// Verdict is the overall decision from a rule or engine.
type Verdict string

const (
	VerdictAllow  Verdict = "allow"
	VerdictReview Verdict = "review"
	VerdictRedact Verdict = "redact"
	VerdictBlock  Verdict = "block"
)

// CombineStrategy controls how multiple rule verdicts are merged.
type CombineStrategy string

const (
	StrategyMostSevere CombineStrategy = "most_severe"
	StrategyBlockOnAny CombineStrategy = "block_on_any"
)

// ErrorStrategy controls the fallback verdict when a rule fails.
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

// RuleType identifies a rule implementation.
type RuleType string

// RuleResult captures a single rule decision.
type RuleResult struct {
	RuleID   string                 `json:"rule_id" yaml:"rule_id"`
	RuleName string                 `json:"rule_name" yaml:"rule_name"`
	RuleType RuleType               `json:"rule_type" yaml:"rule_type"`
	Verdict  Verdict                `json:"verdict" yaml:"verdict"`
	Reason   string                 `json:"reason,omitempty" yaml:"reason,omitempty"`
	Evidence map[string]interface{} `json:"evidence,omitempty" yaml:"evidence,omitempty"`
}

// RuleError captures an evaluation failure for a rule.
type RuleError struct {
	RuleID   string   `json:"rule_id" yaml:"rule_id"`
	RuleName string   `json:"rule_name" yaml:"rule_name"`
	RuleType RuleType `json:"rule_type" yaml:"rule_type"`
	Error    string   `json:"error" yaml:"error"`
}

// Result is the aggregated guardrails decision.
type Result struct {
	Verdict Verdict      `json:"verdict" yaml:"verdict"`
	Reasons []RuleResult `json:"reasons,omitempty" yaml:"reasons,omitempty"`
	Errors  []RuleError  `json:"errors,omitempty" yaml:"errors,omitempty"`
}

// Rule evaluates a single guardrail policy.
type Rule interface {
	ID() string
	Name() string
	Type() RuleType
	Enabled() bool
	Evaluate(ctx context.Context, input Input) (RuleResult, error)
}

// Guardrails is the interface for evaluating input.
type Guardrails interface {
	Evaluate(ctx context.Context, input Input) (Result, error)
}
