package configapply

import (
	"github.com/tingly-dev/tingly-box/internal/server/config"
)

// ApplyConfigResponse is the response for ApplyClaudeConfig
type ApplyConfigResponse struct {
	Success          bool               `json:"success"`
	SettingsResult   config.ApplyResult `json:"settingsResult"`
	OnboardingResult config.ApplyResult `json:"onboardingResult"`
	CreatedFiles     []string           `json:"createdFiles"`
	UpdatedFiles     []string           `json:"updatedFiles"`
	BackupPaths      []string           `json:"backupPaths"`
}

// ApplyOpenCodeConfigResponse is the response for ApplyOpenCodeConfigFromState
type ApplyOpenCodeConfigResponse struct {
	config.ApplyResult
}

// OpenCodeConfigPreviewResponse is the response for GetOpenCodeConfigPreview
type OpenCodeConfigPreviewResponse struct {
	Success    bool   `json:"success"`
	ConfigJSON string `json:"configJson"`
	ScriptWin  string `json:"scriptWindows"`
	ScriptUnix string `json:"scriptUnix"`
	Message    string `json:"message,omitempty"`
}
