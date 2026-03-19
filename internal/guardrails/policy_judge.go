package guardrails

import (
	"context"
	"errors"
)

// PolicyTypeJudge identifies judge-backed policies.
const PolicyTypeJudge PolicyType = "model_judge"

// Judge evaluates input using a model or external service.
type Judge interface {
	Evaluate(ctx context.Context, input Input, cfg ModelJudgeConfig) (JudgeResult, error)
}

// JudgeResult is the outcome from a judge.
type JudgeResult struct {
	Verdict  Verdict                `json:"verdict" yaml:"verdict"`
	Reason   string                 `json:"reason,omitempty" yaml:"reason,omitempty"`
	Evidence map[string]interface{} `json:"evidence,omitempty" yaml:"evidence,omitempty"`
}

// ModelJudgeConfig configures model-based judge policies.
type ModelJudgeConfig struct {
	Model            string        `json:"model,omitempty" yaml:"model,omitempty"`
	Prompt           string        `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	Threshold        float64       `json:"threshold,omitempty" yaml:"threshold,omitempty"`
	Targets          []ContentType `json:"targets,omitempty" yaml:"targets,omitempty"`
	VerdictOnRefuse  Verdict       `json:"verdict_on_refuse,omitempty" yaml:"verdict_on_refuse,omitempty"`
	VerdictOnError   Verdict       `json:"verdict_on_error,omitempty" yaml:"verdict_on_error,omitempty"`
	ReasonOnFallback string        `json:"reason_on_fallback,omitempty" yaml:"reason_on_fallback,omitempty"`
}

// JudgePolicy evaluates policies by delegating to an external judge.
type JudgePolicy struct {
	id      string
	name    string
	enabled bool
	scope   Scope
	config  ModelJudgeConfig
	judge   Judge
}

// NewJudgePolicy creates a judge-backed policy from typed policy data.
func NewJudgePolicy(id, name string, enabled bool, scope Scope, params ModelJudgeConfig, judge Judge) (*JudgePolicy, error) {
	if params.VerdictOnError == "" {
		params.VerdictOnError = VerdictReview
	}
	if params.VerdictOnRefuse == "" {
		params.VerdictOnRefuse = VerdictReview
	}

	return &JudgePolicy{
		id:      id,
		name:    name,
		enabled: enabled,
		scope:   scope,
		config:  params,
		judge:   judge,
	}, nil
}

// ID returns the policy ID.
func (r *JudgePolicy) ID() string {
	return r.id
}

// Name returns the policy name.
func (r *JudgePolicy) Name() string {
	return r.name
}

// Type returns the policy type.
func (r *JudgePolicy) Type() PolicyType {
	return PolicyTypeJudge
}

// Enabled returns whether the policy is enabled.
func (r *JudgePolicy) Enabled() bool {
	return r.enabled
}

// Evaluate calls the judge for a verdict.
func (r *JudgePolicy) Evaluate(ctx context.Context, input Input) (PolicyResult, error) {
	if !r.enabled {
		return PolicyResult{Verdict: VerdictAllow}, nil
	}
	if !r.scope.Matches(input) {
		return PolicyResult{Verdict: VerdictAllow}, nil
	}
	if r.judge == nil {
		return PolicyResult{}, errors.New("judge dependency is not configured")
	}
	if len(r.config.Targets) > 0 && !input.Content.HasAny(r.config.Targets) {
		return PolicyResult{Verdict: VerdictAllow}, nil
	}

	if len(r.config.Targets) > 0 {
		input.Content = input.Content.Filter(r.config.Targets)
	}
	judgeResult, err := r.judge.Evaluate(ctx, input, r.config)
	if err != nil {
		return PolicyResult{}, err
	}

	verdict := judgeResult.Verdict
	if verdict == "" {
		verdict = VerdictAllow
	}

	return PolicyResult{
		PolicyID:   r.id,
		PolicyName: r.name,
		PolicyType: r.Type(),
		Verdict:    verdict,
		Reason:     judgeResult.Reason,
		Evidence:   judgeResult.Evidence,
	}, nil
}
