package agent

import (
	"cmp"
	"fmt"
	"strings"

	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
)

// ClaudeCodeConfig implements AgentConfig for Claude Code
type ClaudeCodeConfig struct{}

// ClaudeCodeParams contains parameters for applying Claude Code configuration
type ClaudeCodeParams struct {
	// BaseURL is the base URL for the Claude API
	BaseURL string

	// APIKey is the authentication token
	APIKey string

	// Model configuration
	ModelConfig ClaudeCodeModelConfig

	// InstallStatusLine installs the status line script
	InstallStatusLine bool

	// ExtraEnv contains additional environment variables beyond the standard ones
	ExtraEnv map[string]string

	// ExtraConfig contains additional config entries for settings.json
	ExtraConfig map[string]interface{}
}

// ClaudeCodeModelConfig defines which models to use for different purposes
type ClaudeCodeModelConfig struct {
	// Default is the default model to use
	Default string

	// Haiku is the model for Haiku requests (optional, uses Default if empty)
	Haiku string

	// Opus is the model for Opus requests (optional, uses Default if empty)
	Opus string

	// Sonnet is the model for Sonnet requests (optional, uses Default if empty)
	Sonnet string

	// SubAgent is the model for sub-agent tasks (optional, uses Default if empty)
	SubAgent string
}

// BuildEnv constructs the complete environment variables map from params
func (p *ClaudeCodeParams) BuildEnv() map[string]string {
	env := map[string]string{
		// Standard settings
		"DISABLE_TELEMETRY":                        "1",
		"DISABLE_ERROR_REPORTING":                  "1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"CLAUDE_CODE_MAX_OUTPUT_TOKENS":            "32000",
		"API_TIMEOUT_MS":                           "3000000",
		"ANTHROPIC_BASE_URL":                       p.BaseURL,
		"ANTHROPIC_AUTH_TOKEN":                     p.APIKey,
	}

	// Model configuration
	defaultModel := cmp.Or(p.ModelConfig.Default, "tingly/cc")

	env["ANTHROPIC_MODEL"] = defaultModel
	env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = cmp.Or(p.ModelConfig.Haiku, defaultModel)
	env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = cmp.Or(p.ModelConfig.Opus, defaultModel)
	env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = cmp.Or(p.ModelConfig.Sonnet, defaultModel)
	env["CLAUDE_CODE_SUBAGENT_MODEL"] = cmp.Or(p.ModelConfig.SubAgent, defaultModel)

	// Add extra env vars
	for k, v := range p.ExtraEnv {
		env[k] = v
	}

	return env
}

// Apply applies Claude Code configuration files
func (c *ClaudeCodeConfig) Apply(paramsInterface interface{}) (*ApplyAgentResult, error) {
	params, ok := paramsInterface.(*ClaudeCodeParams)
	if !ok {
		return nil, fmt.Errorf("invalid params type, expected *ClaudeCodeParams")
	}

	// Build env from params
	env := params.BuildEnv()

	// Apply settings.json
	settingsResult, err := applyClaudeSettings(env, params.InstallStatusLine, params.ExtraConfig)
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

// configFileStatusPrefixes maps the leading words of an ApplyResult.Message
// (e.g. "Created /path/to/file") to the status shown alongside the path.
var configFileStatusPrefixes = []struct {
	prefix string
	status string
}{
	{"Created ", "created"},
	{"Updated ", "updated"},
}

// collectConfigFiles collects the config file paths from apply results
func collectConfigFiles(results ...*serverconfig.ApplyResult) []string {
	var files []string
	for _, r := range results {
		if r == nil {
			continue
		}
		for _, p := range configFileStatusPrefixes {
			rest, ok := strings.CutPrefix(r.Message, p.prefix)
			if !ok {
				continue
			}
			file, _, _ := strings.Cut(rest, " ")
			if file != "" {
				files = append(files, fmt.Sprintf("%s (%s)", file, p.status))
			}
			break
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
