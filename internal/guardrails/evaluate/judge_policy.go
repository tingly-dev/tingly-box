package evaluate

import (
	"context"
	"errors"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

// PolicyTypeJudge identifies judge-backed policies.
const PolicyTypeJudge guardrailscore.PolicyType = "model_judge"

// Judge evaluates input using a model or external service.
type Judge interface {
	Evaluate(ctx context.Context, input guardrailscore.Input, cfg ModelJudgeConfig) (JudgeResult, error)
}

// JudgeResult is the outcome from a judge.
type JudgeResult struct {
	Verdict  guardrailscore.Verdict  `json:"verdict" yaml:"verdict"`
	Reason   string                  `json:"reason,omitempty" yaml:"reason,omitempty"`
	Evidence map[string]interface{}  `json:"evidence,omitempty" yaml:"evidence,omitempty"`
}

// ModelJudgeConfig configures model-based judge policies.
type ModelJudgeConfig struct {
	Model            string                      `json:"model,omitempty" yaml:"model,omitempty"`
	Prompt           string                      `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	Threshold        float64                     `json:"threshold,omitempty" yaml:"threshold,omitempty"`
	Targets          []guardrailscore.ContentType `json:"targets,omitempty" yaml:"targets,omitempty"`
	VerdictOnRefuse  guardrailscore.Verdict      `json:"verdict_on_refuse,omitempty" yaml:"verdict_on_refuse,omitempty"`
	VerdictOnError   guardrailscore.Verdict      `json:"verdict_on_error,omitempty" yaml:"verdict_on_error,omitempty"`
	ReasonOnFallback string                      `json:"reason_on_fallback,omitempty" yaml:"reason_on_fallback,omitempty"`
}

// JudgePolicy evaluates policies by delegating to an external judge.
type JudgePolicy struct {
	id      string
	name    string
	enabled bool
	scope   guardrailscore.Scope
	config  ModelJudgeConfig
	judge   Judge
}

// NewJudgePolicy creates a judge-backed policy from typed policy data.
func NewJudgePolicy(id, name string, enabled bool, scope guardrailscore.Scope, params ModelJudgeConfig, judge Judge) (*JudgePolicy, error) {
	if params.VerdictOnError == "" {
		params.VerdictOnError = guardrailscore.VerdictReview
	}
	if params.VerdictOnRefuse == "" {
		params.VerdictOnRefuse = guardrailscore.VerdictReview
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

func (r *JudgePolicy) ID() string { return r.id }

func (r *JudgePolicy) Name() string { return r.name }

func (r *JudgePolicy) Type() guardrailscore.PolicyType { return PolicyTypeJudge }

func (r *JudgePolicy) Enabled() bool { return r.enabled }

// Evaluate calls the judge for a verdict.
func (r *JudgePolicy) Evaluate(ctx context.Context, input guardrailscore.Input) (guardrailscore.PolicyResult, error) {
	if !r.enabled {
		return guardrailscore.PolicyResult{Verdict: guardrailscore.VerdictAllow}, nil
	}
	if !r.scope.Matches(input) {
		return guardrailscore.PolicyResult{Verdict: guardrailscore.VerdictAllow}, nil
	}
	if r.judge == nil {
		return guardrailscore.PolicyResult{}, errors.New("judge dependency is not configured")
	}
	if len(r.config.Targets) > 0 && !input.Content.HasAny(r.config.Targets) {
		return guardrailscore.PolicyResult{Verdict: guardrailscore.VerdictAllow}, nil
	}

	if len(r.config.Targets) > 0 {
		input.Content = input.Content.Filter(r.config.Targets)
	}
	judgeResult, err := r.judge.Evaluate(ctx, input, r.config)
	if err != nil {
		return guardrailscore.PolicyResult{}, err
	}

	verdict := judgeResult.Verdict
	if verdict == "" {
		verdict = guardrailscore.VerdictAllow
	}

	return guardrailscore.PolicyResult{
		PolicyID:   r.id,
		PolicyName: r.name,
		PolicyType: r.Type(),
		Verdict:    verdict,
		Reason:     judgeResult.Reason,
		Evidence:   judgeResult.Evidence,
	}, nil
}

