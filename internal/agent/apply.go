package agent

import (
	"fmt"
	"os"
	"path/filepath"
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

// applyClaudeCode applies Claude Code configuration.
// When no provider/model is supplied (i.e. no routing service is configured
// yet), the config files are still written and routing-rule sync is skipped
// with a warning — apply should not hard-fail just because rules are unset.
func (aa *AgentApply) applyClaudeCode(req *ApplyAgentRequest) (*ApplyAgentResult, error) {
	result := &ApplyAgentResult{
		AgentType: req.AgentType,
	}

	hasService := req.Provider != "" && req.Model != ""

	if hasService {
		provider, err := aa.config.GetProviderByUUID(req.Provider)
		if err != nil || provider == nil {
			// Provider lookup failed — downgrade to "no service" rather than
			// blocking the file-level apply. Tell the caller via Warnings.
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("provider %s not found; skipping routing rule update", req.Provider))
			hasService = false
		} else {
			result.ProviderName = provider.Name
			result.ProviderUUID = provider.UUID
			result.Model = req.Model
		}
	} else {
		result.Warnings = append(result.Warnings,
			"no routing service configured; applying config files only (routing rules will not be created)")
	}

	// Get base URL and token for Claude settings
	baseURL, apiKey := aa.getBaseURLAndToken()

	// Generate env vars for Claude settings
	env := GenerateClaudeCodeEnv(baseURL, apiKey, req.Unified)

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

	if hasService {
		ruleCreated, ruleUpdated, err := aa.createOrUpdateClaudeCodeRules(result.ProviderUUID, req.Model)
		if err != nil {
			// Surface as a warning so the user still gets the config files;
			// failing the whole apply here would defeat the purpose of the
			// "warn, don't error" contract.
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("failed to create/update routing rules: %v", err))
		} else {
			result.RulesCreated = ruleCreated
			result.RulesUpdated = ruleUpdated
		}
	}

	result.Message = aa.buildResultMessage(result)

	return result, nil
}

// applyOpenCode applies OpenCode configuration.
// Mirrors applyClaudeCode: writes config files unconditionally and only
// creates/updates routing rules when a provider+model are supplied.
func (aa *AgentApply) applyOpenCode(req *ApplyAgentRequest) (*ApplyAgentResult, error) {
	result := &ApplyAgentResult{
		AgentType: req.AgentType,
	}

	hasService := req.Provider != "" && req.Model != ""

	if hasService {
		provider, err := aa.config.GetProviderByUUID(req.Provider)
		if err != nil || provider == nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("provider %s not found; skipping routing rule update", req.Provider))
			hasService = false
		} else {
			result.ProviderName = provider.Name
			result.ProviderUUID = provider.UUID
			result.Model = req.Model
		}
	} else {
		result.Warnings = append(result.Warnings,
			"no routing service configured; applying config files only (routing rules will not be created)")
	}

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

	if hasService {
		ruleCreated, ruleUpdated, err := aa.createOrUpdateOpenCodeRules(result.ProviderUUID, req.Model)
		if err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("failed to create/update routing rules: %v", err))
		} else {
			result.RulesCreated = ruleCreated
			result.RulesUpdated = ruleUpdated
		}
	}

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

// GenerateOpenCodePayload generates the OpenCode configuration payload.
// models is a map of model-name → model config object (map[string]interface{}).
// Pass nil or empty map to use the default single-model layout.
func GenerateOpenCodePayload(configBaseURL, apiKey string, models map[string]interface{}) map[string]interface{} {
	if len(models) == 0 {
		ruleName := "tingly-opencode"
		models = map[string]interface{}{
			ruleName: map[string]interface{}{"name": ruleName},
		}
	}
	providerConfig := map[string]interface{}{
		"tingly-box": map[string]interface{}{
			"name": "tingly-box",
			"npm":  "@ai-sdk/anthropic",
			"options": map[string]interface{}{
				"baseURL": configBaseURL,
				"apiKey":  apiKey,
			},
			"models": models,
		},
	}
	return map[string]interface{}{
		"$schema":  "https://opencode.ai/config.json",
		"provider": providerConfig,
	}
}

// generateOpenCodeConfigPayload generates the configuration payload for OpenCode
// Uses the rule name (tingly-opencode) instead of actual model name
func (aa *AgentApply) generateOpenCodeConfigPayload(configBaseURL, apiKey, _ string) map[string]interface{} {
	return GenerateOpenCodePayload(configBaseURL, apiKey, nil)
}

// applyClaudeSettings applies Claude Code settings.json
func (aa *AgentApply) applyClaudeSettings(env map[string]string, installStatusLine bool) (*serverconfig.ApplyResult, error) {
	var opts []serverconfig.ApplyOption
	if installStatusLine {
		// Install status line script
		_, _, err := serverconfig.InstallStatusLineScript()
		if err != nil {
			return nil, fmt.Errorf("failed to install status line script: %w", err)
		}
		baseURL, _ := aa.getBaseURLAndToken()
		statusLineCmd := fmt.Sprintf("TINGLY_API_URL=%s ~/.claude/tingly-statusline.sh", baseURL)
		statusLine := map[string]any{"type": "command", "command": statusLineCmd}
		opts = append(opts, serverconfig.WithExtra("statusLine", statusLine))
	}

	return serverconfig.ApplyClaudeSettingsFromEnv(env, opts...)
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

// RestoreAgent restores all config files for the given agent type from their
// most recent backup. Each file is handled independently — a missing backup
// for one file does not abort the others; per-file outcomes are summarised in
// the returned RestoreAgentResult.
//
// Routing rules and other in-process state are NOT touched: backups only cover
// the on-disk config files. Re-run `agent apply` afterward to bring rules back
// in sync if needed.
func (aa *AgentApply) RestoreAgent(req *RestoreAgentRequest) (*RestoreAgentResult, error) {
	if !req.AgentType.IsValid() {
		return nil, fmt.Errorf("unknown agent type: %s", req.AgentType)
	}

	info, ok := GetAgentInfo(req.AgentType)
	if !ok {
		return nil, fmt.Errorf("no info registered for agent type: %s", req.AgentType)
	}

	result := &RestoreAgentResult{AgentType: req.AgentType}

	for _, displayPath := range info.ConfigFiles {
		realPath, err := expandUser(displayPath)
		if err != nil {
			result.Failures = append(result.Failures,
				fmt.Sprintf("%s: %v", displayPath, err))
			continue
		}
		r, err := serverconfig.RestoreLatestBackup(realPath)
		if err != nil {
			msg := fmt.Sprintf("%s: %v", displayPath, err)
			if r != nil && r.Message != "" {
				msg = fmt.Sprintf("%s: %s", displayPath, r.Message)
			}
			result.Failures = append(result.Failures, msg)
			continue
		}
		result.RestoredFiles = append(result.RestoredFiles,
			fmt.Sprintf("%s <- %s", displayPath, r.RestoredFrom))
		if r.PreRestoreBackup != "" {
			result.PreRestoreBackups = append(result.PreRestoreBackups, r.PreRestoreBackup)
		}
	}

	result.Success = len(result.RestoredFiles) > 0 && len(result.Failures) == 0
	result.Message = aa.buildRestoreMessage(result)
	return result, nil
}

// expandUser resolves a leading "~" or "~/" in p against the current user's
// home directory. Other paths are returned unchanged.
func expandUser(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		if p == "~" {
			return home, nil
		}
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil
}

// buildRestoreMessage formats a human-readable summary of a restore run.
func (aa *AgentApply) buildRestoreMessage(result *RestoreAgentResult) string {
	var sb strings.Builder
	if result.Success {
		sb.WriteString(fmt.Sprintf("Restored configuration for %s\n", result.AgentType))
	} else if len(result.RestoredFiles) > 0 {
		sb.WriteString(fmt.Sprintf("Partial restore for %s\n", result.AgentType))
	} else {
		sb.WriteString(fmt.Sprintf("Restore failed for %s\n", result.AgentType))
	}

	if len(result.RestoredFiles) > 0 {
		sb.WriteString("\nRestored files:\n")
		for _, f := range result.RestoredFiles {
			sb.WriteString(fmt.Sprintf("  - %s\n", f))
		}
	}
	if len(result.PreRestoreBackups) > 0 {
		sb.WriteString("\nPre-restore safety backups (used to roll back this restore):\n")
		for _, p := range result.PreRestoreBackups {
			sb.WriteString(fmt.Sprintf("  - %s\n", p))
		}
	}
	if len(result.Failures) > 0 {
		sb.WriteString("\nFailures:\n")
		for _, m := range result.Failures {
			sb.WriteString(fmt.Sprintf("  - %s\n", m))
		}
	}
	sb.WriteString("\nNote: routing rules are not part of the backup. Run `agent apply` to resync them if needed.\n")
	return sb.String()
}

// buildResultMessage builds a human-readable result message
func (aa *AgentApply) buildResultMessage(result *ApplyAgentResult) string {
	if !result.Success {
		return "Configuration application failed"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Configuration applied for %s\n", result.AgentType))
	if result.ProviderName != "" {
		sb.WriteString(fmt.Sprintf("Provider: %s\n", result.ProviderName))
	}
	if result.Model != "" {
		sb.WriteString(fmt.Sprintf("Model: %s\n", result.Model))
	}

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

	if len(result.Warnings) > 0 {
		sb.WriteString("\nWarnings:\n")
		for _, w := range result.Warnings {
			sb.WriteString(fmt.Sprintf("  - %s\n", w))
		}
	}

	return sb.String()
}
