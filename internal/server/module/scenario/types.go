package scenario

import (
	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ScenarioFlagUpdateRequest represents the request to update a boolean flag
type ScenarioFlagUpdateRequest struct {
	Value bool `json:"value"`
}

// ScenarioStringFlagUpdateRequest represents the request to update a string flag
type ScenarioStringFlagUpdateRequest struct {
	Value string `json:"value"`
}

// ScenarioIntFlagUpdateRequest represents the request to update an integer flag
type ScenarioIntFlagUpdateRequest struct {
	Value int `json:"value"`
}

// ScenarioUpdateRequest represents the request to update a scenario
type ScenarioUpdateRequest struct {
	Scenario typ.RuleScenario  `json:"scenario" binding:"required" example:"claude_code"`
	Flags    typ.ScenarioFlags `json:"flags" binding:"required"`
}

// ProfileCreateRequest represents the request to create or rename a profile
type ProfileCreateRequest struct {
	Name    string `json:"name" binding:"required"`
	Unified bool   `json:"unified,omitempty"` // Optional, defaults to false (separate mode)
}

// ProfileUpdateRequest represents the request to update a profile
type ProfileUpdateRequest struct {
	Name    string `json:"name,omitempty"`
	Unified *bool  `json:"unified,omitempty"` // Pointer to distinguish zero from unset; nil = no change
}

// ProfileClaudeConfigRequest is the complete desired typed configuration for
// one Claude Code profile. The backend persists only its delta from inheritance.
type ProfileClaudeConfigRequest struct {
	Preferences *agent.ClaudeCodePrefs `json:"preferences" binding:"required"`
	DefaultMode string                 `json:"defaultMode,omitempty"`
}

type ProfileClaudeConfigData struct {
	BasePreferences      agent.ClaudeCodePrefs `json:"basePreferences"`
	Preferences          agent.ClaudeCodePrefs `json:"preferences"`
	InheritedDefaultMode string                `json:"inheritedDefaultMode"`
	DefaultMode          string                `json:"defaultMode"`
	HasOverrides         bool                  `json:"hasOverrides"`
	SettingsPath         string                `json:"settingsPath"`
	SettingsExists       bool                  `json:"settingsExists"`
}

type ProfileClaudeConfigResponse struct {
	Success bool                    `json:"success"`
	Data    ProfileClaudeConfigData `json:"data"`
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

// ScenarioDescriptorsResponse represents the response for listing scenario descriptors
type ScenarioDescriptorsResponse struct {
	Success bool                     `json:"success" example:"true"`
	Data    []typ.ScenarioDescriptor `json:"data"`
}
