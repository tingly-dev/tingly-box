package usecase

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// RuleUseCase implements Rule CRUD. It holds a *serverconfig.Config directly
// (not an AppManager — see .design/usecase-layer.md, "Construction").
type RuleUseCase struct {
	cfg *serverconfig.Config
}

// NewRuleUseCase constructs a RuleUseCase over the given config.
func NewRuleUseCase(cfg *serverconfig.Config) *RuleUseCase {
	return &RuleUseCase{cfg: cfg}
}

// ErrRuleExists means a rule for the same (RequestModel, Scenario) pair
// already exists. Carries the UUID of the existing rule so the caller can
// point the user at it (CLI: "use `rule update`"; TUI: "use Edit";
// a future HTTP handler: 409 with UUID in the body) without this package
// rendering any of that prose itself.
type ErrRuleExists struct {
	RequestModel string
	Scenario     typ.RuleScenario
	UUID         string
}

func (e ErrRuleExists) Error() string {
	return fmt.Sprintf("rule for %q + %q already exists (uuid %s)", e.RequestModel, e.Scenario, e.UUID)
}

// ErrRuleNotFound means no rule exists for the given UUID.
type ErrRuleNotFound struct {
	UUID string
}

func (e ErrRuleNotFound) Error() string {
	return fmt.Sprintf("rule not found: %s", e.UUID)
}

// ListRulesResult is the output of List.
type ListRulesResult struct {
	Rules []typ.Rule `json:"rules"`
}

// List returns every configured rule.
func (uc *RuleUseCase) List() ListRulesResult {
	return ListRulesResult{Rules: uc.cfg.Rules}
}

// GetRuleRequest identifies a rule by UUID.
type GetRuleRequest struct {
	UUID string `json:"uuid"`
}

// GetRuleResult is the output of Get.
type GetRuleResult struct {
	Rule typ.Rule `json:"rule"`
}

// Get returns a rule by UUID, or ErrRuleNotFound.
func (uc *RuleUseCase) Get(req GetRuleRequest) (GetRuleResult, error) {
	rule := uc.cfg.GetRuleByUUID(req.UUID)
	if rule == nil {
		return GetRuleResult{}, ErrRuleNotFound{UUID: req.UUID}
	}
	return GetRuleResult{Rule: *rule}, nil
}

// CreateRuleRequest is the input to Create. Service describes the single
// initial service on the rule — richer multi-service rules are assembled by
// the caller and passed to Create via Services directly if more than one is
// needed (mirrors config_rule.go's "add/update pick a single service" scope,
// but Create accepts the full slice since the DTO has no reason to
// artificially narrow it to one).
type CreateRuleRequest struct {
	Scenario     typ.RuleScenario       `json:"scenario"`
	RequestModel string                 `json:"request_model"`
	Services     []*loadbalance.Service `json:"services"`
}

// CreateRuleResult is the output of Create.
type CreateRuleResult struct {
	Rule typ.Rule `json:"rule"`
}

// Create adds a new rule. Returns ErrRuleExists if a rule for the same
// (RequestModel, Scenario) pair already exists — mirrors the pre-check CLI
// callers do today via GetRuleByRequestModelAndScenario before calling
// AddRule, made into part of the contract instead of a caller-side
// convention every surface has to remember to repeat.
func (uc *RuleUseCase) Create(req CreateRuleRequest) (CreateRuleResult, error) {
	if existing := uc.cfg.GetRuleByRequestModelAndScenario(req.RequestModel, req.Scenario); existing != nil {
		return CreateRuleResult{}, ErrRuleExists{
			RequestModel: req.RequestModel,
			Scenario:     req.Scenario,
			UUID:         existing.UUID,
		}
	}

	rule := typ.Rule{
		UUID:         uuid.New().String(),
		Scenario:     req.Scenario,
		RequestModel: req.RequestModel,
		Services:     req.Services,
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRandom,
			Params: typ.DefaultRandomParams(),
		},
		Active: true,
	}
	if err := uc.cfg.AddRule(rule); err != nil {
		return CreateRuleResult{}, err
	}
	return CreateRuleResult{Rule: rule}, nil
}

// UpdateServiceRequest re-points an existing rule at a different service.
// Everything else on the rule (request model, scenario, flags, tactic)
// stays as-is — mirrors runRuleUpdateService's scope in config_rule.go.
type UpdateServiceRequest struct {
	UUID     string                 `json:"uuid"`
	Services []*loadbalance.Service `json:"services"`
}

// UpdateServiceResult is the output of UpdateService.
type UpdateServiceResult struct {
	Rule typ.Rule `json:"rule"`
}

// UpdateService replaces the Services on an existing rule. Returns
// ErrRuleNotFound if the UUID doesn't match any rule.
func (uc *RuleUseCase) UpdateService(req UpdateServiceRequest) (UpdateServiceResult, error) {
	rule := uc.cfg.GetRuleByUUID(req.UUID)
	if rule == nil {
		return UpdateServiceResult{}, ErrRuleNotFound{UUID: req.UUID}
	}

	updated := *rule
	updated.Services = req.Services

	if err := uc.cfg.UpdateRule(rule.UUID, updated); err != nil {
		return UpdateServiceResult{}, err
	}
	return UpdateServiceResult{Rule: updated}, nil
}

// DeleteRuleRequest identifies the rule to delete.
type DeleteRuleRequest struct {
	UUID string `json:"uuid"`
}

// Delete removes a rule by UUID. Returns ErrRuleNotFound if the UUID
// doesn't match any rule — confirmation (if any) is the caller's concern.
func (uc *RuleUseCase) Delete(req DeleteRuleRequest) error {
	rule := uc.cfg.GetRuleByUUID(req.UUID)
	if rule == nil {
		return ErrRuleNotFound{UUID: req.UUID}
	}
	return uc.cfg.DeleteRule(rule.UUID)
}
