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
