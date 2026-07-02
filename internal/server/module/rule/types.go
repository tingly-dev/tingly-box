package rule

import (
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

// UpdateRuleResponse represents the response for setting/updating a rule.
//
// Data is the full persisted Rule, not a hand-picked subset of fields. The
// backend may normalize a saved rule in ways the client didn't ask for
// (service tiers compacted to a contiguous 0-based sequence, Claude Desktop's
// [1m] request-model suffix synced with the context_1m flag, etc.); echoing
// the complete rule back means every such normalization reaches the client
// automatically, without a matching field having to be added here by hand
// each time. The client should replace its local rule state with this value
// wholesale rather than merging it field-by-field against what it sent.
type UpdateRuleResponse struct {
	Success bool      `json:"success" example:"true"`
	Message string    `json:"message" example:"Rule saved successfully"`
	Data    *typ.Rule `json:"data"`
}

// DeleteRuleResponse represents the response for deleting a rule
type DeleteRuleResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Rule deleted successfully"`
}

// ImportRuleRequest represents request to import a rule from base64 encoded data
type ImportRuleRequest struct {
	Data string `json:"data" binding:"required" description:"Base64 encoded rule export data" example:"TGB64:1.0:..."`
	// OnProviderConflict specifies what to do when a provider already exists.
	// "use" - use existing provider, "skip" - skip this provider, "suffix" - create with suffixed name
	OnProviderConflict string `json:"on_provider_conflict" description:"How to handle provider conflicts" example:"use"`
	// OnRuleConflict specifies what to do when a rule already exists.
	// "skip" - skip import, "update" - update existing rule, "new" - create with new name
	OnRuleConflict string `json:"on_rule_conflict" description:"How to handle rule conflicts" example:"new"`
}

// ImportRuleResponse represents the response for importing a rule
type ImportRuleResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Rule imported successfully"`
	Data    struct {
		RuleCreated      bool           `json:"rule_created" example:"true"`
		RuleUpdated      bool           `json:"rule_updated" example:"false"`
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
