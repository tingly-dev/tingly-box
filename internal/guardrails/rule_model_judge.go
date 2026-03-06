package guardrails

import (
	"context"
	"errors"
	"fmt"
)

// RuleTypeModelJudge delegates verdicts to a judge model.
const RuleTypeModelJudge RuleType = "model_judge"

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

// ModelJudgeConfig configures model-based rules.
type ModelJudgeConfig struct {
	Model            string        `json:"model,omitempty" yaml:"model,omitempty"`
	Prompt           string        `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	Threshold        float64       `json:"threshold,omitempty" yaml:"threshold,omitempty"`
	Targets          []ContentType `json:"targets,omitempty" yaml:"targets,omitempty"`
	VerdictOnRefuse  Verdict       `json:"verdict_on_refuse,omitempty" yaml:"verdict_on_refuse,omitempty"`
	VerdictOnError   Verdict       `json:"verdict_on_error,omitempty" yaml:"verdict_on_error,omitempty"`
	ReasonOnFallback string        `json:"reason_on_fallback,omitempty" yaml:"reason_on_fallback,omitempty"`
}

// ModelJudgeRule calls a judge to produce a verdict.
type ModelJudgeRule struct {
	id      string
	name    string
	enabled bool
	scope   Scope
	config  ModelJudgeConfig
	judge   Judge
}

func init() {
	RegisterRule(RuleTypeModelJudge, newModelJudgeFactory)
}

func newModelJudgeFactory(cfg RuleConfig, deps Dependencies) (Rule, error) {
	return NewModelJudgeRuleFromConfig(cfg, deps.Judge)
}

// NewModelJudgeRuleFromConfig creates a model judge rule from config.
func NewModelJudgeRuleFromConfig(cfg RuleConfig, judge Judge) (*ModelJudgeRule, error) {
	params := ModelJudgeConfig{}
	if err := DecodeParams(cfg.Params, &params); err != nil {
		return nil, fmt.Errorf("decode params: %w", err)
	}

	if params.VerdictOnError == "" {
		params.VerdictOnError = VerdictReview
	}
	if params.VerdictOnRefuse == "" {
		params.VerdictOnRefuse = VerdictReview
	}

	return &ModelJudgeRule{
		id:      cfg.ID,
		name:    cfg.Name,
		enabled: cfg.Enabled,
		scope:   cfg.Scope,
		config:  params,
		judge:   judge,
	}, nil
}

// ID returns the rule ID.
func (r *ModelJudgeRule) ID() string {
	return r.id
}

// Name returns the rule name.
func (r *ModelJudgeRule) Name() string {
	return r.name
}

// Type returns the rule type.
func (r *ModelJudgeRule) Type() RuleType {
	return RuleTypeModelJudge
}

// Enabled returns whether the rule is enabled.
func (r *ModelJudgeRule) Enabled() bool {
	return r.enabled
}

// Evaluate calls the judge for a verdict.
func (r *ModelJudgeRule) Evaluate(ctx context.Context, input Input) (RuleResult, error) {
	if !r.enabled {
		return RuleResult{Verdict: VerdictAllow}, nil
	}
	if !r.scope.Matches(input) {
		return RuleResult{Verdict: VerdictAllow}, nil
	}
	if r.judge == nil {
		return RuleResult{}, errors.New("judge dependency is not configured")
	}
	if len(r.config.Targets) > 0 && !input.Content.HasAny(r.config.Targets) {
		return RuleResult{Verdict: VerdictAllow}, nil
	}

	if len(r.config.Targets) > 0 {
		input.Content = input.Content.Filter(r.config.Targets)
	}
	judgeResult, err := r.judge.Evaluate(ctx, input, r.config)
	if err != nil {
		return RuleResult{}, err
	}

	verdict := judgeResult.Verdict
	if verdict == "" {
		verdict = VerdictAllow
	}

	return RuleResult{
		RuleID:   r.id,
		RuleName: r.name,
		RuleType: r.Type(),
		Verdict:  verdict,
		Reason:   judgeResult.Reason,
		Evidence: judgeResult.Evidence,
	}, nil
}
