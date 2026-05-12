package agent

import (
	"fmt"

	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
)

// ClaudeCodeConfig implements AgentConfig for Claude Code
type ClaudeCodeConfig struct{}

// ClaudeCodeParams contains parameters for applying Claude Code configuration
type ClaudeCodeParams struct {
	// Env is the complete environment variables map for Claude Code settings
	// Caller is responsible for constructing this with appropriate model names
	Env map[string]string

	// InstallStatusLine installs the status line script
	InstallStatusLine bool

	// ExtraConfig contains additional config entries for settings.json
	ExtraConfig map[string]interface{}
}

// Apply applies Claude Code configuration files
func (c *ClaudeCodeConfig) Apply(paramsInterface interface{}) (*ApplyAgentResult, error) {
	params, ok := paramsInterface.(*ClaudeCodeParams)
	if !ok {
		return nil, fmt.Errorf("invalid params type, expected *ClaudeCodeParams")
	}

	// Apply settings.json with provided env
	settingsResult, err := applyClaudeSettings(params.Env, params.InstallStatusLine, params.ExtraConfig)
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
// Deprecated: Use ClaudeCodeConfig.Apply() instead
func ApplyClaudeCode(params *ClaudeCodeParams) (*ApplyAgentResult, error) {
	config := &ClaudeCodeConfig{}
	return config.Apply(params)
}

// applyClaudeSettings applies Claude Code settings.json
func applyClaudeSettings(env map[string]string, installStatusLine bool, extraConfig map[string]interface{}) (*serverconfig.ApplyResult, error) {
	var opts []serverconfig.ApplyOption
	if installStatusLine {
		// Install status line script
		_, _, err := serverconfig.InstallStatusLineScript()
		if err != nil {
			return nil, fmt.Errorf("failed to install status line script: %w", err)
		}
		statusLineCmd := "~/.claude/tingly-statusline.sh"
		statusLine := map[string]any{"type": "command", "command": statusLineCmd}
		opts = append(opts, serverconfig.WithExtra("statusLine", statusLine))
	}

	// Add extra config entries
	for key, value := range extraConfig {
		opts = append(opts, serverconfig.WithExtra(key, value))
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
		if len(msg) > 8 && containsPrefix(msg[8:], "Created ") {
			// Find the file path after "Created "
			file := extractFilePath(msg[8:])
			if file != "" {
				files = append(files, file+" (created)")
			}
		} else if len(msg) > 8 && containsPrefix(msg[8:], "Updated ") {
			file := extractFilePath(msg[8:])
			if file != "" {
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

// Helper functions to avoid importing strings package

func containsPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func extractFilePath(s string) string {
	// Find the end of the file path (first space after prefix)
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '(' {
			return s[:i]
		}
	}
	return ""
}
