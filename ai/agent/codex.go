package agent

import (
	"fmt"

	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
)

// CodexConfig implements AgentConfig for Codex
type CodexConfig struct{}

// CodexParams contains parameters for applying Codex configuration
type CodexParams struct {
	// CodexBaseURL is the base URL for Codex API endpoint. Ignored when
	// AuthMode is CodexAuthChatGPT (native OAuth uses OpenAI directly and we
	// leave the user's own ~/.codex/config.toml untouched).
	CodexBaseURL string

	// APIKey is the authentication token used in gateway / apikey mode.
	APIKey string

	// Models is a list of model names for the Codex profiles
	// Caller is responsible for collecting and deduplicating these
	Models []string

	// Prefs holds the typed, whitelisted, user-tunable Codex config.toml keys
	// (see serverconfig.CodexPrefs). nil means "use built-in defaults".
	Prefs *serverconfig.CodexPrefs

	// WriteCatalog controls whether ~/.codex/tingly-model-catalog.json is
	// written and model_catalog_json is set in config.toml. When true, Codex's
	// /model picker can list tingly-served models. Defaults to true for the CLI
	// path; the HTTP handler derives it from the request.
	WriteCatalog bool

	// AuthMode selects how ~/.codex/auth.json is populated. Empty defaults to
	// CodexAuthAPIKey (gateway). CodexAuthChatGPT exports native OAuth tokens
	// for direct codex-to-OpenAI use; tingly-box stops managing the tokens
	// after the one-shot write.
	AuthMode serverconfig.CodexAuthMode

	// ChatGPTTokens carries the OAuth credentials used when AuthMode is
	// CodexAuthChatGPT. Ignored otherwise.
	ChatGPTTokens *serverconfig.CodexChatGPTTokens
}

// Apply applies Codex CLI configuration
func (c *CodexConfig) Apply(paramsInterface interface{}) (*ApplyAgentResult, error) {
	params, ok := paramsInterface.(*CodexParams)
	if !ok {
		return nil, fmt.Errorf("invalid params type, expected *CodexParams")
	}

	result := &ApplyAgentResult{
		AgentType: AgentTypeCodex,
		Success:   true,
	}

	// In ChatGPT mode the user is going direct-to-OpenAI; we don't touch
	// config.toml — their existing model_provider / base_url stay intact.
	if params.AuthMode != serverconfig.CodexAuthChatGPT {
		configResult, err := serverconfig.ApplyCodexConfig(params.CodexBaseURL, params.Models, params.Prefs, params.WriteCatalog)
		if err != nil {
			return nil, fmt.Errorf("failed to apply Codex config: %w", err)
		}
		result.Success = result.Success && configResult.Success
		if configResult.Success {
			suffix := " (updated)"
			if configResult.Created {
				suffix = " (created)"
			}
			result.ConfigFiles = append(result.ConfigFiles, "~/.codex/config.toml"+suffix)
		}
		if configResult.BackupPath != "" {
			result.BackupPaths = append(result.BackupPaths, configResult.BackupPath)
		}
	}

	// Apply auth.json
	authResult, err := serverconfig.ApplyCodexAuth(params.AuthMode, params.APIKey, params.ChatGPTTokens)
	if err != nil {
		return nil, fmt.Errorf("failed to apply Codex auth: %w", err)
	}
	result.Success = result.Success && authResult.Success

	if authResult.Success {
		suffix := " (updated)"
		if authResult.Created {
			suffix = " (created)"
		}
		result.ConfigFiles = append(result.ConfigFiles, "~/.codex/auth.json"+suffix)
	}
	if authResult.BackupPath != "" {
		result.BackupPaths = append(result.BackupPaths, authResult.BackupPath)
	}

	return result, nil
}

// Restore restores Codex configuration from backup
func (c *CodexConfig) Restore() (*RestoreAgentResult, error) {
	return RestoreAgent(AgentTypeCodex)
}

// ApplyCodex applies Codex CLI configuration.
// Deprecated: Use CodexConfig.Apply() instead
func ApplyCodex(params *CodexParams) (*ApplyAgentResult, error) {
	config := &CodexConfig{}
	return config.Apply(params)
}
