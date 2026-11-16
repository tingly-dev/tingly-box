package rule

import (
	"github.com/tingly-dev/tingly-box/internal/routing/smartrouting"
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
		// Flags echoes the rule's persisted feature flags. Creation seeds
		// per-scenario defaults (e.g. team rules get claude_code_compat +
		// clean_header), so the client must read them back from here to reflect
		// the true stored state without a follow-up refresh.
		Flags typ.RuleFlags `json:"flags,omitempty"`
	} `json:"data"`
}

// DeleteRuleResponse represents the response for deleting a rule
type DeleteRuleResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Rule deleted successfully"`
}

// ReorderRulesRequest represents the request to apply a new list order to one
// scenario's rules. ordered_uuids names that scenario's rules in their new
// order; rules of the scenario missing from the list keep their current
// relative order and follow the named ones. The order is hard state: it drives
// /v1/models listing, same-tier match priority and client config generation.
type ReorderRulesRequest struct {
	Scenario     string   `json:"scenario" example:"openai"`
	OrderedUUIDs []string `json:"ordered_uuids"`
}

// FlagRegistryResponse exposes the catalog of supported rule-level flags so the
// UI can render an extension catalog without hard-coding the list.
type FlagRegistryResponse struct {
	Success bool           `json:"success" example:"true"`
	Data    []typ.FlagSpec `json:"data"`
}
