package agent

import (
	"fmt"
	"strings"

	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
)

// ClaudeCodeConfig implements AgentConfig for Claude Code
type ClaudeCodeConfig struct{}

// ClaudeCodeParams contains parameters for applying Claude Code configuration
type ClaudeCodeParams struct {
	// BaseURL is the base URL for the tingly-box server
	BaseURL string
	// APIKey is the authentication token
	APIKey string
	// Unified specifies unified mode (single config for all models)
	Unified bool
	// InstallStatusLine installs the status line script
	InstallStatusLine bool
}

// Apply applies Claude Code configuration files
func (c *ClaudeCodeConfig) Apply(paramsInterface interface{}) (*ApplyAgentResult, error) {
	params, ok := paramsInterface.(*ClaudeCodeParams)
	if !ok {
		return nil, fmt.Errorf("invalid params type, expected *ClaudeCodeParams")
	}

	// Generate env vars for Claude settings
	env := GenerateClaudeCodeEnv(params.BaseURL, params.APIKey, params.Unified)

	// Apply settings.json
	settingsResult, err := applyClaudeSettings(env, params.InstallStatusLine)
	if err != nil {
		return nil, fmt.Errorf("failed to apply Claude settings: %w", err)
	}

	// Apply .claude.json
	onboardingResult, err := applyClaudeOnboarding()
	if err != nil {
		return nil, fmt.Errorf("failed to apply Claude onboarding: %w", err)
	}

	// Collect results
	result := &ApplyAgentResult{
		AgentType:   AgentTypeClaudeCode,
		Success:     settingsResult.Success && onboardingResult.Success,
		ConfigFiles: collectConfigFiles(settingsResult, onboardingResult),
		BackupPaths: collectBackupPaths(settingsResult, onboardingResult),
	}

	return result, nil
}

// Restore restores Claude Code configuration from backup
func (c *ClaudeCodeConfig) Restore() (*RestoreAgentResult, error) {
	return RestoreAgent(AgentTypeClaudeCode)
}

// ApplyClaudeCode applies Claude Code configuration files.
// This function does NOT handle routing rules - that's done by the caller.
// Deprecated: Use ClaudeCodeConfig.Apply() instead
func ApplyClaudeCode(params *ClaudeCodeParams) (*ApplyAgentResult, error) {
	config := &ClaudeCodeConfig{}
	return config.Apply(params)
}

// GenerateClaudeCodeEnv generates environment variables for Claude Code settings.
// unified=true means all model slots point to "tingly/cc"; false uses separate cc-* models.
func GenerateClaudeCodeEnv(baseURL, apiKey string, unified bool) map[string]string {
	env := map[string]string{
		"DISABLE_TELEMETRY":                        "1",
		"DISABLE_ERROR_REPORTING":                  "1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"CLAUDE_CODE_MAX_OUTPUT_TOKENS":            "32000",
		"API_TIMEOUT_MS":                           "3000000",
		"ANTHROPIC_BASE_URL":                       baseURL + "/tingly/claude_code",
		"ANTHROPIC_AUTH_TOKEN":                     apiKey,
	}

	if unified {
		// Unified mode - all point to same model
		env["ANTHROPIC_MODEL"] = "tingly/cc"
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = "tingly/cc"
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = "tingly/cc"
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = "tingly/cc"
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = "tingly/cc"
	} else {
		// Separate mode - different models for different purposes
		env["ANTHROPIC_MODEL"] = "tingly/cc-default"
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = "tingly/cc-haiku"
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = "tingly/cc-opus"
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = "tingly/cc-sonnet"
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = "tingly/cc-subagent"
	}

	return env
}

// applyClaudeSettings applies Claude Code settings.json
func applyClaudeSettings(env map[string]string, installStatusLine bool) (*serverconfig.ApplyResult, error) {
	var opts []serverconfig.ApplyOption
	if installStatusLine {
		// Install status line script
		_, _, err := serverconfig.InstallStatusLineScript()
		if err != nil {
			return nil, fmt.Errorf("failed to install status line script: %w", err)
		}
		statusLineCmd := fmt.Sprintf("~/.claude/tingly-statusline.sh")
		statusLine := map[string]any{"type": "command", "command": statusLineCmd}
		opts = append(opts, serverconfig.WithExtra("statusLine", statusLine))
	}

	return serverconfig.ApplyClaudeSettingsFromEnv(env, opts...)
}

// applyClaudeOnboarding applies Claude Code .claude.json
func applyClaudeOnboarding() (*serverconfig.ApplyResult, error) {
	payload := map[string]interface{}{
		"hasCompletedOnboarding": true,
	}
	return serverconfig.ApplyClaudeOnboarding(payload)
}

// collectConfigFiles collects the config file paths from apply results
func collectConfigFiles(results ...*serverconfig.ApplyResult) []string {
	var files []string
	for _, r := range results {
		if r == nil {
			continue
		}
		msg := r.Message
		if strings.Contains(msg, "Created ") {
			parts := strings.SplitN(msg, "Created ", 2)
			if len(parts) > 1 {
				file := strings.SplitN(parts[1], " ", 2)[0]
				files = append(files, file+" (created)")
			}
		} else if strings.Contains(msg, "Updated ") {
			parts := strings.SplitN(msg, "Updated ", 2)
			if len(parts) > 1 {
				file := strings.SplitN(parts[1], " ", 2)[0]
				files = append(files, file+" (updated)")
			}
		}
	}
	return files
}

// collectBackupPaths collects backup paths from apply results
func collectBackupPaths(results ...*serverconfig.ApplyResult) []string {
	var paths []string
	for _, r := range results {
		if r != nil && r.BackupPath != "" {
			paths = append(paths, r.BackupPath)
		}
	}
	return paths
}
