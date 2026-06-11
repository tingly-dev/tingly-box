package server

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
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
	templateManager := cfg.GetTemplateManager()

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

		// Track provider for template lookup
		var primaryProvider *typ.Provider

		if len(services) > 0 {
			providerNames := make([]string, 0, len(services))
			for i := range services {
				svc := services[i]
				if svc.Active {
					provider, err := cfg.GetProviderByUUID(svc.Provider)
					if err == nil {
						if primaryProvider == nil {
							primaryProvider = provider
						}
						providerNames = append(providerNames, provider.Name)
					}
				}
			}
			if len(providerNames) > 0 {
				displayName += fmt.Sprintf(" (via %v)", providerNames)
			}
		}

		// Get model description from template if available
		var description string
		var maxInputTokens int
		var maxTokens int
		if templateManager != nil && primaryProvider != nil {
			if tmpl, err := templateManager.GetTemplate(primaryProvider.Name); err == nil && tmpl != nil {
				for _, modelInfo := range tmpl.Models {
					if modelInfo.ID == rule.RequestModel {
						description = modelInfo.Description
						maxInputTokens = modelInfo.Context
						maxTokens = modelInfo.MaxOutput
						break
					}
				}
			}
		}

		// Build capabilities from Anthropic API provider data
		var caps *ModelCapabilities
		if primaryProvider != nil && templateManager != nil {
			if tmpl, err := templateManager.GetTemplate(primaryProvider.Name); err == nil && tmpl != nil {
				// Only populate capabilities for known Anthropic providers
				if tmpl.VendorFamily == "anthropic" {
					yes := CapabilitySupport{Supported: true}
					no := CapabilitySupport{Supported: false}
					caps = &ModelCapabilities{
						Batch:             no,
						Citations:         yes,
						CodeExecution:     yes,
						ImageInput:        yes,
						PDFInput:          yes,
						StructuredOutputs: yes,
						ContextManagement: &ContextManagementCapability{
							Supported:             true,
							ClearThinking20251015: yes,
							ClearToolUses20250919: yes,
							Compact20260112:       yes,
						},
						Effort: &EffortCapability{
							Supported: true,
							Low:       yes,
							Medium:    yes,
							High:      yes,
							XHigh:     yes,
							Max:       yes,
						},
						Thinking: &ThinkingCapability{
							Supported: true,
							Types: &ThinkingTypes{
								Adaptive: yes,
								Enabled:  yes,
							},
						},
					}
				}
			}
		}

		logrus.Warnf("We do not set capabilities, even set: %v", caps)

		models = append(models, AnthropicModel{
			ID:          rule.RequestModel,
			CreatedAt:   "2024-01-01T00:00:00Z",
			DisplayName: displayName,
			Type:        "model",
			// Capabilities:   caps,
			MaxInputTokens: maxInputTokens,
			MaxTokens:      maxTokens,
			Description:    description,
			AuthType:       string(primaryAuthTypeForRule(cfg, rule)),
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
