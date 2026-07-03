package server

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

type (
	// CapabilitySupport indicates whether a capability is supported.
	CapabilitySupport struct {
		Supported bool `json:"supported"`
	}

	// ContextManagementCapability describes context management support.
	ContextManagementCapability struct {
		Supported             bool              `json:"supported"`
		ClearThinking20251015 CapabilitySupport `json:"clear_thinking_20251015,omitempty"`
		ClearToolUses20250919 CapabilitySupport `json:"clear_tool_uses_20250919,omitempty"`
		Compact20260112       CapabilitySupport `json:"compact_20260112,omitempty"`
	}

	// EffortCapability describes reasoning_effort support and levels.
	EffortCapability struct {
		Supported bool              `json:"supported"`
		Low       CapabilitySupport `json:"low,omitempty"`
		Medium    CapabilitySupport `json:"medium,omitempty"`
		High      CapabilitySupport `json:"high,omitempty"`
		XHigh     CapabilitySupport `json:"xhigh,omitempty"`
		Max       CapabilitySupport `json:"max,omitempty"`
	}

	// ThinkingTypes describes supported thinking type configurations.
	ThinkingTypes struct {
		Adaptive CapabilitySupport `json:"adaptive,omitempty"`
		Enabled  CapabilitySupport `json:"enabled,omitempty"`
	}

	// ThinkingCapability describes thinking support.
	ThinkingCapability struct {
		Supported bool           `json:"supported"`
		Types     *ThinkingTypes `json:"types,omitempty"`
	}

	// ModelCapabilities maps to Anthropic's ModelCapabilities in /v1/models.
	ModelCapabilities struct {
		Batch             CapabilitySupport            `json:"batch"`
		Citations         CapabilitySupport            `json:"citations"`
		CodeExecution     CapabilitySupport            `json:"code_execution"`
		ContextManagement *ContextManagementCapability `json:"context_management,omitempty"`
		Effort            *EffortCapability            `json:"effort,omitempty"`
		ImageInput        CapabilitySupport            `json:"image_input"`
		PDFInput          CapabilitySupport            `json:"pdf_input"`
		StructuredOutputs CapabilitySupport            `json:"structured_outputs"`
		Thinking          *ThinkingCapability          `json:"thinking,omitempty"`
	}

	// AnthropicModel maps to Anthropic's native /v1/models response format.
	AnthropicModel struct {
		ID             string             `json:"id"`
		CreatedAt      string             `json:"created_at"`
		DisplayName    string             `json:"display_name"`
		Type           string             `json:"type"`
		Capabilities   *ModelCapabilities `json:"capabilities,omitempty"`
		MaxInputTokens int                `json:"max_input_tokens,omitempty"`
		MaxTokens      int                `json:"max_tokens,omitempty"`
		// Description is a tingly-box extension (not in Anthropic's wire format)
		// consumed by the frontend to show model description in the model picker.
		Description string `json:"description,omitempty"`
		// AuthType is a tingly-box extension (not in Anthropic's wire format)
		// consumed by the frontend to order model picker entries:
		// oauth -> api_key -> vmodel.
		AuthType string `json:"auth_type,omitempty"`
	}
	AnthropicModelsResponse struct {
		Data    []AnthropicModel `json:"data"`
		FirstID string           `json:"first_id"`
		HasMore bool             `json:"has_more"`
		LastID  string           `json:"last_id"`
	}
)

// HandleAnthropicListModels handles Anthropic v1 models endpoint
func (ph *ProtocolHandler) HandleAnthropicListModels(c *gin.Context) {
	ph.anthropicListModelsWithScenario(c, nil)
}

// AnthropicListModelsForScenario handles scenario-scoped model listing for Anthropic format
func (ph *ProtocolHandler) AnthropicListModelsForScenario(c *gin.Context, scenario typ.RuleScenario) {
	ph.anthropicListModelsWithScenario(c, &scenario)
}

func (ph *ProtocolHandler) anthropicListModelsWithScenario(c *gin.Context, scenario *typ.RuleScenario) {
	cfg := ph.deps.Config
	if cfg == nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, ErrorResponse{
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
		if scenario != nil && !ShouldIncludeRuleInModelList(*scenario, rule.GetScenario()) {
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
			AuthType:       string(PrimaryAuthTypeForRule(cfg, rule)),
		})
	}

	sort.SliceStable(models, func(i, j int) bool {
		return AuthTypeSortWeight(typ.AuthType(models[i].AuthType)) <
			AuthTypeSortWeight(typ.AuthType(models[j].AuthType))
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
