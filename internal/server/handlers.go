package server

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func (s *Server) determineRule(modelName string) (*typ.Rule, error) {
	c := s.config
	if c != nil && c.IsRequestModel(modelName) {

		// Get the Rule for this specific request model using the same method as middleware
		uuid := c.GetUUIDByRequestModel(modelName)
		rules := c.GetRequestConfigs()
		var rule *typ.Rule
		for i := range rules {
			if rules[i].UUID == uuid && rules[i].Active {
				rule = &rules[i] // Get pointer to actual rule in config
				return rule, nil
			}
		}
	}

	return nil, fmt.Errorf("provider or model not configured for request model '%s'", modelName)
}

// probeSyntheticRuleUUID marks the throwaway rule built for an
// X-Tingly-Probe-Service request — it has no persisted identity. Alias for
// aimodel.ProbeSyntheticRuleUUID, which now owns the value (Step 7) since
// protocol_dispatch.go's setProbeUpstreamHeaders needs it there too.
const probeSyntheticRuleUUID = ProbeSyntheticRuleUUID

func (s *Server) determineRuleWithScenario(ctx *gin.Context, scenario typ.RuleScenario, modelName string) (*typ.Rule, error) {
	// X-Tingly-Probe-Rule: load a specific rule by UUID (for applying its flags
	// while service selection is overridden by X-Tingly-Probe-Service).
	if ruleUUID := ctx.GetHeader("X-Tingly-Probe-Rule"); ruleUUID != "" {
		if rule := s.config.GetRuleByUUID(ruleUUID); rule != nil {
			return rule, nil
		}
		return nil, fmt.Errorf("probe rule not found: %s", ruleUUID)
	}

	// X-Tingly-Probe-Service: no matching rule needed — build a minimal synthetic
	// rule so the handler can proceed with service selection pinned by the header.
	if probeService := ctx.GetHeader("X-Tingly-Probe-Service"); probeService != "" {
		if providerUUID, model, ok := strings.Cut(probeService, ":"); ok {
			svc := &loadbalance.Service{Provider: providerUUID, Model: model, Active: true}
			return &typ.Rule{
				UUID:         probeSyntheticRuleUUID,
				Scenario:     scenario,
				RequestModel: model,
				Services:     []*loadbalance.Service{svc},
				Active:       true,
			}, nil
		}
	}

	cfg := s.config
	if cfg != nil {
		// Use the new MatchRuleByModelAndScenario which supports wildcard matching
		rule := cfg.MatchRuleByModelAndScenario(modelName, scenario)
		if rule != nil && rule.Active {
			return rule, nil
		}
		// Enterprise runtime context is already authorized by TBE.
		// If endpoint scenario has no matching rule, allow lookup by model across scenarios.
		if isEnterpriseContextPresent(ctx) {
			for _, anyRule := range cfg.GetRequestConfigs() {
				if anyRule.Active && anyRule.RequestModel == modelName {
					return &anyRule, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("provider or model not configured for request model '%s'", modelName)
}
