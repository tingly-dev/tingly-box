package scenario

import "github.com/tingly-dev/tingly-box/internal/typ"

// ScenarioFlagUpdateRequest represents the request to update a boolean flag
type ScenarioFlagUpdateRequest struct {
	Value bool `json:"value"`
}

// ScenarioStringFlagUpdateRequest represents the request to update a string flag
type ScenarioStringFlagUpdateRequest struct {
	Value string `json:"value"`
}

// ScenarioUpdateRequest represents the request to update a scenario
type ScenarioUpdateRequest struct {
	Scenario typ.RuleScenario  `json:"scenario" binding:"required" example:"claude_code"`
	Flags    typ.ScenarioFlags `json:"flags" binding:"required"`
}

// ScenariosResponse represents the response for getting all scenarios
type ScenariosResponse struct {
	Success bool                 `json:"success" example:"true"`
	Data    []typ.ScenarioConfig `json:"data"`
}

// ScenarioResponse represents the response for a single scenario
type ScenarioResponse struct {
	Success bool               `json:"success" example:"true"`
	Data    typ.ScenarioConfig `json:"data"`
}

// ScenarioFlagResponse represents the response for a scenario flag
type ScenarioFlagResponse struct {
	Success bool `json:"success" example:"true"`
	Data    struct {
		Scenario typ.RuleScenario `json:"scenario" example:"claude_code"`
		Flag     string           `json:"flag" example:"unified"`
		Value    bool             `json:"value" example:"true"`
	} `json:"data"`
}

// ScenarioUpdateResponse represents the response for updating scenario
type ScenarioUpdateResponse struct {
	Success bool               `json:"success" example:"true"`
	Message string             `json:"message" example:"Scenario config saved successfully"`
	Data    typ.ScenarioConfig `json:"data"`
}
