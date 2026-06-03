package agent

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ClaudeCodeRequestModels defines all request models for Claude Code scenario
// When applying claude-code agent, all these rules should be updated for convenience
var ClaudeCodeRequestModels = []string{
	"tingly/cc", // General model (for unified mode)
	"tingly/cc-haiku",
	"tingly/cc-sonnet",
	"tingly/cc-opus",
	"tingly/cc-default",
	"tingly/cc-subagent",
}

// OpenCodeRequestModels defines all request models for OpenCode scenario
var OpenCodeRequestModels = []string{
	"tingly-opencode",
}

// CodexRequestModels defines the default request models for the Codex scenario.
// Users typically add additional rules with their own request_model names; this
// list only seeds the default rule when none exists yet.
var CodexRequestModels = []string{
	"tingly-codex",
}

// createOrUpdateClaudeCodeRules creates or updates all Claude Code rules.
// For convenience, all tingly/cc-* rules are updated with the same provider + model.
func (aa *AgentApply) createOrUpdateClaudeCodeRules(providerUUID, model string) (int, int, error) {
	return aa.createOrUpdateRulesForScenario(typ.ScenarioClaudeCode, "Claude Code", ClaudeCodeRequestModels, providerUUID, model)
}

// createOrUpdateOpenCodeRules creates or updates OpenCode rules.
func (aa *AgentApply) createOrUpdateOpenCodeRules(providerUUID, model string) (int, int, error) {
	return aa.createOrUpdateRulesForScenario(typ.ScenarioOpenCode, "OpenCode", OpenCodeRequestModels, providerUUID, model)
}

// createOrUpdateCodexRules creates or updates the default Codex rule (tingly-codex).
// Other Codex rules the user has added by hand are left alone — Codex apply just
// needs *some* active rule to seed the generated config.toml.
func (aa *AgentApply) createOrUpdateCodexRules(providerUUID, model string) (int, int, error) {
	return aa.createOrUpdateRulesForScenario(typ.ScenarioCodex, "Codex", CodexRequestModels, providerUUID, model)
}

// createOrUpdateRulesForScenario applies a single provider+model to every request
// model in the list, creating or updating the corresponding routing rule.
func (aa *AgentApply) createOrUpdateRulesForScenario(
	scenario typ.RuleScenario,
	label string,
	requestModels []string,
	providerUUID, model string,
) (int, int, error) {
	created := 0
	updated := 0

	service := &loadbalance.Service{
		Active:   true,
		Provider: providerUUID,
		Model:    model,
	}

	for _, requestModel := range requestModels {
		ruleCreated, ruleUpdated, err := aa.createOrUpdateRule(
			scenario,
			requestModel,
			service,
			fmt.Sprintf("%s - %s routing", label, requestModel),
		)
		if err != nil {
			return created, updated, fmt.Errorf("failed to update rule %s: %w", requestModel, err)
		}
		if ruleCreated {
			created++
		}
		if ruleUpdated {
			updated++
		}
	}

	return created, updated, nil
}

// createOrUpdateRule creates or updates a single rule
// This follows the server's rule management pattern from internal/server/config/config.go
func (aa *AgentApply) createOrUpdateRule(
	scenario typ.RuleScenario,
	requestModel string,
	service *loadbalance.Service,
	description string,
) (bool, bool, error) {
	// Check if rule already exists with this RequestModel + Scenario
	existingRule := aa.config.GetRuleByRequestModelAndScenario(requestModel, scenario)

	if existingRule != nil {
		// Update existing rule
		// Replace services with the new one
		existingRule.Services = []*loadbalance.Service{service}
		existingRule.Active = true

		if err := aa.config.UpdateRule(existingRule.UUID, *existingRule); err != nil {
			return false, false, fmt.Errorf("failed to update rule: %w", err)
		}
		return false, true, nil
	}

	// Create new rule
	rule := typ.Rule{
		UUID:          uuid.New().String(),
		Scenario:      scenario,
		RequestModel:  requestModel,
		ResponseModel: "",
		Description:   description,
		Services:      []*loadbalance.Service{service},
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRandom,
			Params: typ.DefaultRandomParams(),
		},
		Active: true,
	}

	if err := aa.config.AddRule(rule); err != nil {
		return false, false, fmt.Errorf("failed to add rule: %w", err)
	}

	return true, false, nil
}
