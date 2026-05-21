package server

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// OpenAIListModels handles the /v1/models endpoint (OpenAI compatible)
func (s *Server) OpenAIListModels(c *gin.Context) {
	s.openAIListModelsWithScenario(c, nil)
}

// OpenAIListModelsForScenario handles scenario-scoped model listing for OpenAI format
func (s *Server) OpenAIListModelsForScenario(c *gin.Context, scenario typ.RuleScenario) {
	s.openAIListModelsWithScenario(c, &scenario)
}

func (s *Server) openAIListModelsWithScenario(c *gin.Context, scenario *typ.RuleScenario) {
	cfg := s.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Config not available",
				Type:    "internal_error",
			},
		})
		return
	}

	rules := cfg.GetRequestConfigs()

	var models []OpenAIModel
	for _, rule := range rules {
		if !rule.Active {
			continue
		}
		if scenario != nil && !shouldIncludeRuleInModelList(*scenario, rule.GetScenario()) {
			continue
		}

		// Get timestamp from provider's LastUpdated field
		var created int64
		services := rule.GetServices()
		providerDesc := make([]string, 0, len(services))
		for i := range services {
			svc := services[i]
			// Skip nil services (defensive check after DB migration)
			if svc == nil {
				logrus.Debugf("Skipping nil service in rule %s during model list", rule.UUID)
				continue
			}
			if svc.Active {
				provider, err := cfg.GetProviderByUUID(svc.Provider)
				if err == nil {
					providerDesc = append(providerDesc, provider.Name)
					// Parse LastUpdated timestamp if available
					if provider.LastUpdated != "" {
						if t, err := time.Parse(time.RFC3339, provider.LastUpdated); err == nil {
							created = t.Unix()
						}
					}
				} else {
					providerDesc = append(providerDesc, svc.Provider)
				}
			}
		}

		// Build owned_by field
		ownedBy := "tingly-box"
		if len(providerDesc) > 0 {
			ownedBy += " via " + fmt.Sprintf("%v", providerDesc)
		}

		models = append(models, OpenAIModel{
			ID:       rule.RequestModel,
			Object:   "model",
			Created:  created,
			OwnedBy:  ownedBy,
			AuthType: string(primaryAuthTypeForRule(cfg, rule)),
		})
	}

	sort.SliceStable(models, func(i, j int) bool {
		return authTypeSortWeight(typ.AuthType(models[i].AuthType)) <
			authTypeSortWeight(typ.AuthType(models[j].AuthType))
	})

	c.JSON(http.StatusOK, OpenAIModelsResponse{
		Object: "list",
		Data:   models,
	})
}

// ListModelsByScenario handles the /v1/models endpoint for scenario-based routing
func (s *Server) ListModelsByScenario(c *gin.Context) {
	scenario := c.Param("scenario")

	// Convert string to RuleScenario and validate
	scenarioType := typ.RuleScenario(scenario)
	if !isValidRuleScenario(scenarioType) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("invalid scenario: %s", scenario),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Route to appropriate handler based on scenario
	switch scenarioType {
	case typ.ScenarioAnthropic, typ.ScenarioClaudeCode, typ.ScenarioClaudeDesktop:
		s.AnthropicListModelsForScenario(c, scenarioType)
	default:
		// OpenAI is the default
		s.OpenAIListModelsForScenario(c, scenarioType)
	}
}
