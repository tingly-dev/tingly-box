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

// createOrUpdateClaudeCodeRules creates or updates all Claude Code rules
// For convenience, all tingly/cc-* rules are updated with the same provider + model
func (aa *AgentApply) createOrUpdateClaudeCodeRules(providerUUID, model string) (int, int, error) {
	created := 0
	updated := 0

	// Create service with the selected provider + model
	service := &loadbalance.Service{
		Active:   true,
		Provider: providerUUID,
		Model:    model,
	}

	// Update all Claude Code request models
	for _, requestModel := range ClaudeCodeRequestModels {
		ruleCreated, ruleUpdated, err := aa.createOrUpdateRule(
			typ.ScenarioClaudeCode,
			requestModel,
			service,
			fmt.Sprintf("Claude Code - %s routing", requestModel),
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

// createOrUpdateOpenCodeRules creates or updates OpenCode rules
func (aa *AgentApply) createOrUpdateOpenCodeRules(providerUUID, model string) (int, int, error) {
	created := 0
	updated := 0

	// Create service with the selected provider + model
	service := &loadbalance.Service{
		Active:   true,
		Provider: providerUUID,
		Model:    model,
	}

	// Update OpenCode request model
	for _, requestModel := range OpenCodeRequestModels {
		ruleCreated, ruleUpdated, err := aa.createOrUpdateRule(
			typ.ScenarioOpenCode,
			requestModel,
			service,
			fmt.Sprintf("OpenCode - %s routing", requestModel),
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
			Type:   loadbalance.TacticRoundRobin,
			Params: typ.DefaultRoundRobinParams(),
		},
		Active: true,
	}

	if err := aa.config.AddRule(rule); err != nil {
		return false, false, fmt.Errorf("failed to add rule: %w", err)
	}

	return true, false, nil
}
