package rule

import (
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// =============================================
// Rule API Types
// =============================================

// RulesResponse represents the response for getting all rules
type RulesResponse struct {
	Success bool        `json:"success" example:"true"`
	Data    interface{} `json:"data"`
}

// RuleResponse represents a rule configuration response
type RuleResponse struct {
	Success bool      `json:"success" example:"true"`
	Data    *typ.Rule `json:"data"`
}

// CreateRuleRequest represents the request to create a rule
type CreateRuleRequest typ.Rule

// UpdateRuleRequest represents the request to set/update a rule
type UpdateRuleRequest typ.Rule

// UpdateRuleResponse represents the response for setting/updating a rule
type UpdateRuleResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Rule saved successfully"`
	Data    struct {
		UUID          string                      `json:"uuid"`
		Scenario      string                      `json:"scenario" example:"openai"`
		RequestModel  string                      `json:"request_model" example:"gpt-3.5-turbo"`
		ResponseModel string                      `json:"response_model" example:"gpt-3.5-turbo"`
		Description   string                      `json:"description" example:"My rule description"`
		Provider      string                      `json:"provider" example:"openai"`
		DefaultModel  string                      `json:"default_model" example:"gpt-3.5-turbo"`
		Active        bool                        `json:"active" example:"true"`
		SmartEnabled  bool                        `json:"smart_enabled" example:"false"`
		SmartRouting  []smartrouting.SmartRouting `json:"smart_routing,omitempty"`
	} `json:"data"`
}

// DeleteRuleResponse represents the response for deleting a rule
type DeleteRuleResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Rule deleted successfully"`
}

// ImportRuleRequest represents request to import providers from base64/JSONL encoded export data.
// Despite the endpoint name, only providers are imported — no rule is created or updated.
// See .design/ for background: dataio export/import is provider-only.
type ImportRuleRequest struct {
	Data string `json:"data" binding:"required" description:"Base64 encoded provider export data" example:"TGB64:1.0:..."`
	// OnProviderConflict specifies what to do when a provider already exists.
	// "use" - use existing provider, "skip" - skip this provider, "suffix" - create with suffixed name
	OnProviderConflict string `json:"on_provider_conflict" description:"How to handle provider conflicts" example:"use"`
}

// ImportRuleResponse represents the response for importing providers
type ImportRuleResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Providers imported successfully"`
	Data    struct {
		ProvidersCreated int            `json:"providers_created" example:"1"`
		ProvidersUsed    int            `json:"providers_used" example:"0"`
		Providers        []ProviderInfo `json:"providers,omitempty"`
	} `json:"data"`
}

// ProviderInfo contains basic information about an imported or used provider
type ProviderInfo struct {
	UUID   string `json:"uuid" example:"123e4567-e89b-12d3-a456-426614174000"`
	Name   string `json:"name" example:"openai"`
	Action string `json:"action" example:"created"` // "created", "used", "skipped"
}

// FlagRegistryResponse exposes the catalog of supported rule-level flags so the
// UI can render an extension catalog without hard-coding the list.
type FlagRegistryResponse struct {
	Success bool           `json:"success" example:"true"`
	Data    []typ.FlagSpec `json:"data"`
}
