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

// OpenAIModel represents a model in OpenAI's models API format
type OpenAIModel struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Created     int64  `json:"created"`
	OwnedBy     string `json:"owned_by"`
	Description string `json:"description,omitempty"` // Model description
	Context     int    `json:"context,omitempty"`     // Max context window
	MaxOutput   int    `json:"max_output,omitempty"`  // Max output tokens
	// AuthType reflects the primary backing provider's auth type. It is
	// non-standard (OpenAI's models API has no such field) and consumed by
	// the tingly-box frontend to order model picker entries:
	// oauth -> api_key -> vmodel.
	AuthType string `json:"auth_type,omitempty"`
}

// OpenAIModelsResponse represents OpenAI's models API response format
type OpenAIModelsResponse struct {
	Object string        `json:"object"`
	Data   []OpenAIModel `json:"data"`
}

// HandleOpenAIListModels handles the /v1/models endpoint (OpenAI compatible)
func (ph *ProtocolHandler) HandleOpenAIListModels(c *gin.Context) {
	ph.openAIListModelsWithScenario(c, nil)
}

// OpenAIListModelsForScenario handles scenario-scoped model listing for OpenAI format
func (ph *ProtocolHandler) OpenAIListModelsForScenario(c *gin.Context, scenario typ.RuleScenario) {
	ph.openAIListModelsWithScenario(c, &scenario)
}

func (ph *ProtocolHandler) openAIListModelsWithScenario(c *gin.Context, scenario *typ.RuleScenario) {
	cfg := ph.deps.Config
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

	var models []OpenAIModel
	for _, rule := range rules {
		if !rule.Active {
			continue
		}
		if scenario != nil && !ShouldIncludeRuleInModelList(*scenario, rule.GetScenario()) {
			continue
		}

		// Get timestamp from provider's LastUpdated field
		var created int64
		services := rule.GetServices()
		providerDesc := make([]string, 0, len(services))

		// Track provider for template lookup
		var primaryProvider *typ.Provider

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
					if primaryProvider == nil {
						primaryProvider = provider
					}
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

		// Get model description from template if available
		var description string
		var context int
		var maxOutput int
		if templateManager != nil && primaryProvider != nil {
			if tmpl, err := templateManager.GetTemplate(primaryProvider.Name); err == nil && tmpl != nil {
				for _, modelInfo := range tmpl.Models {
					if modelInfo.ID == rule.RequestModel {
						description = modelInfo.Description
						context = modelInfo.Context
						maxOutput = modelInfo.MaxOutput
						break
					}
				}
			}
		}

		models = append(models, OpenAIModel{
			ID:          rule.RequestModel,
			Object:      "model",
			Created:     created,
			OwnedBy:     ownedBy,
			Description: description,
			Context:     context,
			MaxOutput:   maxOutput,
			AuthType:    string(PrimaryAuthTypeForRule(cfg, rule)),
		})
	}

	sort.SliceStable(models, func(i, j int) bool {
		return AuthTypeSortWeight(typ.AuthType(models[i].AuthType)) <
			AuthTypeSortWeight(typ.AuthType(models[j].AuthType))
	})

	c.JSON(http.StatusOK, OpenAIModelsResponse{
		Object: "list",
		Data:   models,
	})
}

// ListModelsByScenario handles the /v1/models endpoint for scenario-based routing
func (ph *ProtocolHandler) ListModelsByScenario(c *gin.Context) {
	scenario := c.Param("scenario")

	// Convert string to RuleScenario and validate
	scenarioType := typ.RuleScenario(scenario)
	if !IsValidRuleScenario(scenarioType) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("invalid scenario: %s", scenario),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Route to appropriate handler based on scenario
	switch scenarioType.Base() {
	case typ.ScenarioAnthropic, typ.ScenarioClaudeCode, typ.ScenarioClaudeDesktop:
		ph.AnthropicListModelsForScenario(c, scenarioType)
	default:
		// OpenAI is the default
		ph.OpenAIListModelsForScenario(c, scenarioType)
	}
}
