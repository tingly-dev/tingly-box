package agent

import (
	"fmt"

	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
)

// CodexConfig implements AgentConfig for Codex
type CodexConfig struct{}

// CodexParams contains parameters for applying Codex configuration
type CodexParams struct {
	// CodexBaseURL is the base URL for Codex API endpoint
	CodexBaseURL string

	// APIKey is the authentication token
	APIKey string

	// Models is a list of model names for the Codex profiles
	// Caller is responsible for collecting and deduplicating these
	Models []string
}

// Apply applies Codex CLI configuration
func (c *CodexConfig) Apply(paramsInterface interface{}) (*ApplyAgentResult, error) {
	params, ok := paramsInterface.(*CodexParams)
	if !ok {
		return nil, fmt.Errorf("invalid params type, expected *CodexParams")
	}

	// Apply config.toml
	configResult, err := serverconfig.ApplyCodexConfig(params.CodexBaseURL, params.Models)
	if err != nil {
		return nil, fmt.Errorf("failed to apply Codex config: %w", err)
	}

	// Apply auth.json
	authResult, err := serverconfig.ApplyCodexAuth(params.APIKey)
	if err != nil {
		return nil, fmt.Errorf("failed to apply Codex auth: %w", err)
	}

	result := &ApplyAgentResult{
		AgentType: AgentTypeCodex,
		Success:   configResult.Success && authResult.Success,
	}

	// Build config files list
	if configResult.Success {
		suffix := " (updated)"
		if configResult.Created {
			suffix = " (created)"
		}
		result.ConfigFiles = append(result.ConfigFiles, "~/.codex/config.toml"+suffix)
	}
	if authResult.Success {
		suffix := " (updated)"
		if authResult.Created {
			suffix = " (created)"
		}
		result.ConfigFiles = append(result.ConfigFiles, "~/.codex/auth.json"+suffix)
	}

	if configResult.BackupPath != "" {
		result.BackupPaths = append(result.BackupPaths, configResult.BackupPath)
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
