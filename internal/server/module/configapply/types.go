package configapply

import (
	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/server/config"
)

// ApplyClaudeConfigRequest is the request body for ApplyClaudeConfig.
//
// `preferences` is the source of truth: each field's JSON tag is the
// Claude Code env var it controls, and ToEnv emits exactly the map
// written under "env" in ~/.claude/settings.json. `installStatusLine`
// is orthogonal — it just toggles the statusLine stanza in settings.json.
type ApplyClaudeConfigRequest struct {
	InstallStatusLine bool                   `json:"installStatusLine"`
	Preferences       *agent.ClaudeCodePrefs `json:"preferences"`
	DefaultMode       string                 `json:"defaultMode,omitempty"`
}

const DefaultClaudeCodeDefaultMode = "acceptEdits"

var validClaudeCodeDefaultModes = map[string]struct{}{
	"acceptEdits":       {},
	"bypassPermissions": {},
	"default":           {},
	"delegate":          {},
	"dontAsk":           {},
	"plan":              {},
	"auto":              {},
}

func normalizeClaudeCodeDefaultMode(mode string) (string, bool) {
	if mode == "" {
		return DefaultClaudeCodeDefaultMode, true
	}
	_, ok := validClaudeCodeDefaultModes[mode]
	return mode, ok
}

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

// ApplyCodexConfigRequest is the request body for the Codex apply and preview
// endpoints. `preferences` is the typed, whitelisted set of Codex config.toml
// keys (see config.CodexPrefs). nil means "use built-in defaults".
// `writeCatalog` controls whether ~/.codex/tingly-model-catalog.json is written
// and model_catalog_json is set in config.toml. nil defaults to true.
type ApplyCodexConfigRequest struct {
	Preferences  *config.CodexPrefs `json:"preferences"`
	WriteCatalog *bool              `json:"writeCatalog"`

	// AuthMode selects how ~/.codex/auth.json is populated. "" / "apikey"
	// writes the gateway key (existing behavior); "chatgpt" exports the
	// OAuth tokens of the provider identified by OAuthProviderUUID so codex
	// CLI talks directly to OpenAI. tingly-box stops managing the tokens
	// after a chatgpt-mode apply — codex CLI owns refresh from then on.
	AuthMode string `json:"authMode,omitempty"`

	// OAuthProviderUUID identifies the Codex OAuth provider whose tokens
	// should be exported. Required when AuthMode == "chatgpt".
	OAuthProviderUUID string `json:"oauthProviderUuid,omitempty"`
}

// ApplyCodexConfigResponse is the response for ApplyCodexConfigFromState.
type ApplyCodexConfigResponse struct {
	Success        bool               `json:"success"`
	ConfigResult   config.ApplyResult `json:"configResult"`
	AuthResult     config.ApplyResult `json:"authResult"`
	CatalogWritten bool               `json:"catalogWritten"`
	Models         []string           `json:"models"`
	Message        string             `json:"message,omitempty"`
}

// CodexConfigPreviewResponse is the response for GetCodexConfigPreview.
type CodexConfigPreviewResponse struct {
	Success     bool     `json:"success"`
	ConfigToml  string   `json:"configToml"`
	AuthJson    string   `json:"authJson"`
	CatalogJson string   `json:"catalogJson,omitempty"`
	Models      []string `json:"models"`
	Message     string   `json:"message,omitempty"`
}

// RestoreConfigResponse is the response for the restore endpoints. It mirrors
// the agent.RestoreAgentResult so callers can drive UI from the same data
// the CLI prints.
type RestoreConfigResponse struct {
	Success           bool     `json:"success"`
	AgentType         string   `json:"agentType"`
	RestoredFiles     []string `json:"restoredFiles"`
	PreRestoreBackups []string `json:"preRestoreBackups"`
	Failures          []string `json:"failures,omitempty"`
	Message           string   `json:"message"`
}
