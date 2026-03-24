package agent

import (
	"fmt"
	"strings"

	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
)

// AgentApply handles agent configuration application
// This is shared logic used by both CLI and HTTP handlers
type AgentApply struct {
	config *serverconfig.Config
	host   string
}

// NewAgentApply creates a new AgentApply instance
func NewAgentApply(cfg *serverconfig.Config, host string) *AgentApply {
	return &AgentApply{
		config: cfg,
		host:   host,
	}
}

// ApplyAgent applies configuration for the specified agent type
func (aa *AgentApply) ApplyAgent(req *ApplyAgentRequest) (*ApplyAgentResult, error) {
	// Validate agent type
	if !req.AgentType.IsValid() {
		return nil, fmt.Errorf("unknown agent type: %s", req.AgentType)
	}

	// Dispatch to specific handler
	switch req.AgentType {
	case AgentTypeClaudeCode:
		return aa.applyClaudeCode(req)
	case AgentTypeOpenCode:
		return aa.applyOpenCode(req)
	default:
		return nil, fmt.Errorf("agent type %s not implemented", req.AgentType)
	}
}

// applyClaudeCode applies Claude Code configuration
func (aa *AgentApply) applyClaudeCode(req *ApplyAgentRequest) (*ApplyAgentResult, error) {
	result := &ApplyAgentResult{
		AgentType: req.AgentType,
	}

	// Get provider
	provider, err := aa.config.GetProviderByUUID(req.Provider)
	if err != nil {
		return nil, fmt.Errorf("provider not found: %w", err)
	}
	if provider == nil {
		return nil, fmt.Errorf("provider %s not found", req.Provider)
	}
	result.ProviderName = provider.Name
	result.ProviderUUID = provider.UUID
	result.Model = req.Model

	// Get base URL and token for Claude settings
	baseURL, apiKey := aa.getBaseURLAndToken()

	// Generate env vars for Claude settings
	env := aa.generateClaudeCodeEnv(baseURL, apiKey, req.Model, req.Unified)

	// Apply settings.json
	settingsResult, err := aa.applyClaudeSettings(env, req.InstallStatusLine)
	if err != nil {
		return nil, fmt.Errorf("failed to apply Claude settings: %w", err)
	}

	// Apply .claude.json
	onboardingResult, err := aa.applyClaudeOnboarding()
	if err != nil {
		return nil, fmt.Errorf("failed to apply Claude onboarding: %w", err)
	}

	// Collect results
	result.Success = settingsResult.Success && onboardingResult.Success
	result.ConfigFiles = aa.collectConfigFiles(settingsResult, onboardingResult)
	result.BackupPaths = aa.collectBackupPaths(settingsResult, onboardingResult)

	// Create or update routing rules (all tingly/cc-* rules for convenience)
	ruleCreated, ruleUpdated, err := aa.createOrUpdateClaudeCodeRules(provider.UUID, req.Model)
	if err != nil {
		return nil, fmt.Errorf("failed to create/update routing rules: %w", err)
	}
	result.RulesCreated = ruleCreated
	result.RulesUpdated = ruleUpdated

	result.Message = aa.buildResultMessage(result)

	return result, nil
}

// applyOpenCode applies OpenCode configuration
func (aa *AgentApply) applyOpenCode(req *ApplyAgentRequest) (*ApplyAgentResult, error) {
	result := &ApplyAgentResult{
		AgentType: req.AgentType,
	}

	// Get provider
	provider, err := aa.config.GetProviderByUUID(req.Provider)
	if err != nil {
		return nil, fmt.Errorf("provider not found: %w", err)
	}
	if provider == nil {
		return nil, fmt.Errorf("provider %s not found", req.Provider)
	}
	result.ProviderName = provider.Name
	result.ProviderUUID = provider.UUID
	result.Model = req.Model

	// Get base URL and token for OpenCode config
	baseURL, apiKey := aa.getBaseURLAndToken()
	configBaseURL := baseURL + "/tingly/opencode"

	// Generate OpenCode config payload
	payload := aa.generateOpenCodeConfigPayload(configBaseURL, apiKey, req.Model)

	// Apply OpenCode config
	applyResult, err := aa.applyOpenCodeConfig(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to apply OpenCode config: %w", err)
	}

	// Collect results
	result.Success = applyResult.Success
	if applyResult.Success {
		result.ConfigFiles = []string{"~/.config/opencode/opencode.json"}
		if applyResult.Created {
			result.ConfigFiles = append(result.ConfigFiles, " (created)")
		} else {
			result.ConfigFiles = append(result.ConfigFiles, " (updated)")
		}
	}
	if applyResult.BackupPath != "" {
		result.BackupPaths = []string{applyResult.BackupPath}
	}

	// Create or update routing rules (tingly-opencode)
	ruleCreated, ruleUpdated, err := aa.createOrUpdateOpenCodeRules(provider.UUID, req.Model)
	if err != nil {
		return nil, fmt.Errorf("failed to create/update routing rules: %w", err)
	}
	result.RulesCreated = ruleCreated
	result.RulesUpdated = ruleUpdated

	result.Message = aa.buildResultMessage(result)

	return result, nil
}

// getBaseURLAndToken returns the base URL and API token for configuration
func (aa *AgentApply) getBaseURLAndToken() (string, string) {
	port := aa.config.ServerPort
	if port == 0 {
		port = 12580
	}
	baseURL := fmt.Sprintf("http://%s:%d", aa.host, port)
	apiKey := aa.config.GetModelToken()
	return baseURL, apiKey
}

// generateClaudeCodeEnv generates environment variables for Claude Code settings
func (aa *AgentApply) generateClaudeCodeEnv(baseURL, apiKey, model string, unified bool) map[string]string {
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

// generateOpenCodeConfigPayload generates the configuration payload for OpenCode
// Uses the rule name (tingly-opencode) instead of actual model name
func (aa *AgentApply) generateOpenCodeConfigPayload(configBaseURL, apiKey, _ string) map[string]interface{} {
	// Use rule name as the model identifier
	ruleName := "tingly-opencode"

	providerConfig := map[string]interface{}{
		"tingly-box": map[string]interface{}{
			"name": "tingly-box",
			"npm":  "@ai-sdk/anthropic",
			"options": map[string]interface{}{
				"baseURL": configBaseURL,
				"apiKey":  apiKey,
			},
			"models": map[string]interface{}{
				"model": map[string]interface{}{
					"name": ruleName,
				},
			},
		},
	}

	return map[string]interface{}{
		"$schema":  "https://opencode.ai/config.json",
		"provider": providerConfig,
	}
}

// applyClaudeSettings applies Claude Code settings.json
func (aa *AgentApply) applyClaudeSettings(env map[string]string, installStatusLine bool) (*serverconfig.ApplyResult, error) {
	var extras []serverconfig.KV
	if installStatusLine {
		// Install status line script
		_, _, err := serverconfig.InstallStatusLineScript()
		if err != nil {
			return nil, fmt.Errorf("failed to install status line script: %w", err)
		}
		statusLine := map[string]any{"type": "command", "command": "~/.claude/tingly-statusline.sh"}
		extras = append(extras, serverconfig.KV{Key: "statusLine", Value: statusLine})
	}

	return serverconfig.ApplyClaudeSettingsFromEnv(env, extras...)
}

// applyClaudeOnboarding applies Claude Code .claude.json
func (aa *AgentApply) applyClaudeOnboarding() (*serverconfig.ApplyResult, error) {
	payload := map[string]interface{}{
		"hasCompletedOnboarding": true,
	}
	return serverconfig.ApplyClaudeOnboarding(payload)
}

// applyOpenCodeConfig applies OpenCode configuration
func (aa *AgentApply) applyOpenCodeConfig(payload map[string]interface{}) (*serverconfig.ApplyResult, error) {
	return serverconfig.ApplyOpenCodeConfig(payload)
}

// collectConfigFiles collects the config file paths from apply results
func (aa *AgentApply) collectConfigFiles(results ...*serverconfig.ApplyResult) []string {
	var files []string
	for _, r := range results {
		if r == nil {
			continue
		}
		// Extract file paths from the message
		// Message format: "Created ~/.claude/settings.json" or "Updated ~/.claude/settings.json (backup: ...)"
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
func (aa *AgentApply) collectBackupPaths(results ...*serverconfig.ApplyResult) []string {
	var paths []string
	for _, r := range results {
		if r != nil && r.BackupPath != "" {
			paths = append(paths, r.BackupPath)
		}
	}
	return paths
}

// buildResultMessage builds a human-readable result message
func (aa *AgentApply) buildResultMessage(result *ApplyAgentResult) string {
	if !result.Success {
		return "Configuration application failed"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Configuration applied for %s\n", result.AgentType))
	sb.WriteString(fmt.Sprintf("Provider: %s\n", result.ProviderName))
	sb.WriteString(fmt.Sprintf("Model: %s\n", result.Model))

	if len(result.ConfigFiles) > 0 {
		sb.WriteString("\nFiles modified:\n")
		for _, f := range result.ConfigFiles {
			sb.WriteString(fmt.Sprintf("  - %s\n", f))
		}
	}

	if result.RulesCreated > 0 {
		sb.WriteString(fmt.Sprintf("\nRouting rules created: %d\n", result.RulesCreated))
	}
	if result.RulesUpdated > 0 {
		sb.WriteString(fmt.Sprintf("Routing rules updated: %d\n", result.RulesUpdated))
	}

	if len(result.BackupPaths) > 0 {
		sb.WriteString("\nBackups:\n")
		for _, p := range result.BackupPaths {
			sb.WriteString(fmt.Sprintf("  - %s\n", p))
		}
	}

	return sb.String()
}
