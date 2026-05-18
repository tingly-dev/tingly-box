package server

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AnthropicListModels handles Anthropic v1 models endpoint
func (s *Server) AnthropicListModels(c *gin.Context) {
	s.anthropicListModelsWithScenario(c, nil)
}

// AnthropicListModelsForScenario handles scenario-scoped model listing for Anthropic format
func (s *Server) AnthropicListModelsForScenario(c *gin.Context, scenario typ.RuleScenario) {
	s.anthropicListModelsWithScenario(c, &scenario)
}

func (s *Server) anthropicListModelsWithScenario(c *gin.Context, scenario *typ.RuleScenario) {
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

	var models []AnthropicModel
	for _, rule := range rules {
		if !rule.Active {
			continue
		}
		if scenario != nil && !shouldIncludeRuleInModelList(*scenario, rule.GetScenario()) {
			continue
		}

		// Build display name with provider info
		displayName := rule.RequestModel
		services := rule.GetServices()
		if len(services) > 0 {
			providerNames := make([]string, 0, len(services))
			for i := range services {
				svc := services[i]
				if svc.Active {
					provider, err := cfg.GetProviderByUUID(svc.Provider)
					if err == nil {
						providerNames = append(providerNames, provider.Name)
					}
				}
			}
			if len(providerNames) > 0 {
				displayName += fmt.Sprintf(" (via %v)", providerNames)
			}
		}

		models = append(models, AnthropicModel{
			ID:          rule.RequestModel,
			CreatedAt:   "2024-01-01T00:00:00Z",
			DisplayName: displayName,
			Type:        "model",
			AuthType:    string(primaryAuthTypeForRule(cfg, rule)),
		})
	}

	sort.SliceStable(models, func(i, j int) bool {
		return authTypeSortWeight(typ.AuthType(models[i].AuthType)) <
			authTypeSortWeight(typ.AuthType(models[j].AuthType))
	})

	firstID := ""
	lastID := ""
	if len(models) > 0 {
		firstID = models[0].ID
		lastID = models[len(models)-1].ID
	}

	c.JSON(http.StatusOK, AnthropicModelsResponse{
		Data:    models,
		FirstID: firstID,
		HasMore: false,
		LastID:  lastID,
	})
}
