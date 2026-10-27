package protocolserver

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/config"
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
	// ContextWindow is the context window the vendor publishes (Tingly Box's
	// own field, in both listings). MaxInputTokens / MaxTokens follow the
	// names and meaning of Anthropic's Models API (OpenAI's Model object has
	// no such fields): the most input one request may carry, which is below
	// ContextWindow when the vendor's window counts output too (gpt-5: 400K
	// window, 272K input), and the most output.
	ContextWindow  int `json:"context_window,omitempty"`
	MaxInputTokens int `json:"max_input_tokens,omitempty"`
	MaxTokens      int `json:"max_tokens,omitempty"`
	// CodexWindows is on the Codex list only (Tingly Box's own field): the
	// context window, ceiling and source that "Auto" writes into Codex's model
	// catalog for this model, with the rule's 1M flag off and on. They follow
	// the route (config.CodexAutoContextWindow), so they can differ from the
	// catalog figures above. The rule editor's Context chip and 1M menu show
	// them.
	CodexWindows *config.CodexAutoWindows `json:"codex_windows,omitempty"`
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
		writeErrorResponse(c, http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Config not available",
				Type:    "internal_error",
			},
		})
		return
	}

	rules := cfg.GetRequestConfigs()
	var codexWindows map[string]config.CodexAutoWindows
	if scenario != nil && *scenario == typ.ScenarioCodex {
		codexWindows = config.ResolveCodexAutoWindows(cfg)
	}

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
					// Parse LastUpdated timestamp if available
					if provider.LastUpdated != "" {
						if t, err := time.Parse(time.RFC3339, provider.LastUpdated); err == nil {
							created = t.Unix()
						}
					}
				}
			}
		}

		// owned_by is always "tingly-box" — never disclose the upstream
		// provider name here.
		ownedBy := "tingly-box"

		// Model details come from the catalog entry of the upstream model
		// this rule routes to.
		info, _ := ruleModelInfo(cfg, rule, ai.APIStyleOpenAI)

		model := OpenAIModel{
			ID:             rule.RequestModel,
			Object:         "model",
			Created:        created,
			OwnedBy:        ownedBy,
			Description:    info.Description,
			Context:        info.Context,
			MaxOutput:      info.MaxOutput,
			ContextWindow:  info.Context,
			MaxInputTokens: info.InputLimit(),
			MaxTokens:      info.MaxOutput,
			AuthType:       string(PrimaryAuthTypeForRule(cfg, rule)),
		}
		if w, ok := codexWindows[strings.TrimSpace(rule.RequestModel)]; ok && rule.GetScenario() == typ.ScenarioCodex {
			model.CodexWindows = &w
		}
		models = append(models, model)
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
