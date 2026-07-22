package agent

import (
	"cmp"
	"fmt"
	"strings"

	aiagent "github.com/tingly-dev/tingly-box/ai/agent"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Re-export types from ai/agent for backward compatibility
type (
	// AgentType represents the type of AI agent to configure
	AgentType = aiagent.AgentType

	// ApplyAgentRequest represents a request to apply agent configuration
	ApplyAgentRequest = aiagent.ApplyAgentRequest

	// ApplyAgentResult represents the result of applying agent configuration
	ApplyAgentResult = aiagent.ApplyAgentResult

	// RestoreAgentRequest represents a request to restore agent configuration
	RestoreAgentRequest = aiagent.RestoreAgentRequest

	// RestoreAgentResult represents the result of restoring agent configuration
	RestoreAgentResult = aiagent.RestoreAgentResult

	// AgentInfo provides information about an agent type
	AgentInfo = aiagent.AgentInfo
)

// Re-export constants for backward compatibility
const (
	// AgentTypeClaudeCode represents Claude Code agent
	AgentTypeClaudeCode = aiagent.AgentTypeClaudeCode

	// AgentTypeOpenCode represents OpenCode IDE extension
	AgentTypeOpenCode = aiagent.AgentTypeOpenCode

	// AgentTypeCodex represents the OpenAI Codex CLI
	AgentTypeCodex = aiagent.AgentTypeCodex
)

// Re-export functions for backward compatibility
var (
	// ParseAgentType parses an agent type string, supporting aliases
	ParseAgentType = aiagent.ParseAgentType

	// ListAgentInfo returns information about all supported agent types
	ListAgentInfo = aiagent.ListAgentInfo

	// GetAgentInfo returns information about a specific agent type
	GetAgentInfo = aiagent.GetAgentInfo
)

// AgentApply handles agent configuration with routing rules (Tingly-Box specific)
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

// ApplyAgent applies configuration including routing rules
// This is the main entry point for Tingly-Box agent configuration
func (aa *AgentApply) ApplyAgent(req *ApplyAgentRequest) (*ApplyAgentResult, error) {
	// Validate agent type
	if !req.AgentType.IsValid() {
		return nil, fmt.Errorf("unknown agent type: %s", req.AgentType)
	}

	// 1. Apply config files using ai/agent
	var fileResult *ApplyAgentResult
	var err error

	baseURL, apiKey := aa.getBaseURLAndToken()

	switch req.AgentType {
	case AgentTypeClaudeCode:
		config, ok := aiagent.DefaultRegistry.Get(req.AgentType)
		if !ok {
			return nil, fmt.Errorf("claude code config not registered")
		}
		// Build env vars with business logic
		modelConfig := BuildClaudeCodeModelConfig(req.Unified)
		fileResult, err = config.Apply(&aiagent.ClaudeCodeParams{
			BaseURL:           baseURL + "/tingly/claude_code",
			APIKey:            apiKey,
			ModelConfig:       modelConfig,
			InstallStatusLine: req.InstallStatusLine,
			ExtraEnv:          nil,
			ExtraConfig:       nil,
		})
	case AgentTypeOpenCode:
		config, ok := aiagent.DefaultRegistry.Get(req.AgentType)
		if !ok {
			return nil, fmt.Errorf("opencode config not registered")
		}
		configBaseURL := baseURL + "/tingly/opencode"
		// Build config object with business logic
		openCodeConfig := BuildOpenCodeConfig(configBaseURL, apiKey, nil)
		fileResult, err = config.Apply(&aiagent.OpenCodeParams{
			Config: openCodeConfig,
		})
	case AgentTypeCodex:
		config, ok := aiagent.DefaultRegistry.Get(req.AgentType)
		if !ok {
			return nil, fmt.Errorf("codex config not registered")
		}
		codexBaseURL := baseURL + "/tingly/codex"
		// Collect models with business logic
		rawModels := aa.collectCodexRuleModels()
		models := CollectCodexModels(rawModels)
		fileResult, err = config.Apply(&aiagent.CodexParams{
			CodexBaseURL: codexBaseURL,
			APIKey:       apiKey,
			Models:       models,
			Prefs:        serverconfig.DefaultCodexPrefs(),
			WriteCatalog: true,
		})
	}

	if err != nil {
		return nil, err
	}

	// 2. Apply routing rules (Tingly-specific)
	if req.Provider != "" && req.Model != "" {
		provider, err := aa.config.GetProviderByUUID(req.Provider)
		if err != nil || provider == nil {
			fileResult.Warnings = append(fileResult.Warnings,
				fmt.Sprintf("provider %s not found; skipping routing rule update", req.Provider))
		} else {
			fileResult.ProviderName = provider.Name
			fileResult.ProviderUUID = provider.UUID
			fileResult.Model = req.Model

			ruleCreated, ruleUpdated, err := aa.createOrUpdateRules(req.AgentType, req.Provider, req.Model)
			if err != nil {
				fileResult.Warnings = append(fileResult.Warnings,
					fmt.Sprintf("failed to create/update routing rules: %v", err))
			} else {
				fileResult.RulesCreated = ruleCreated
				fileResult.RulesUpdated = ruleUpdated
			}
		}
	}

	fileResult.Message = aa.buildResultMessage(fileResult)
	return fileResult, nil
}

// createOrUpdateRules creates or updates routing rules for the given agent type
func (aa *AgentApply) createOrUpdateRules(agentType AgentType, providerUUID, model string) (int, int, error) {
	switch agentType {
	case AgentTypeClaudeCode:
		return aa.createOrUpdateClaudeCodeRules(providerUUID, model)
	case AgentTypeOpenCode:
		return aa.createOrUpdateOpenCodeRules(providerUUID, model)
	case AgentTypeCodex:
		return aa.createOrUpdateCodexRules(providerUUID, model)
	default:
		return 0, 0, fmt.Errorf("agent type %s not implemented", agentType)
	}
}

// RestoreAgent restores configuration files from backup
func (aa *AgentApply) RestoreAgent(req *RestoreAgentRequest) (*RestoreAgentResult, error) {
	config, ok := aiagent.DefaultRegistry.Get(req.AgentType)
	if !ok {
		return nil, fmt.Errorf("no config registered for agent type: %s", req.AgentType)
	}
	return config.Restore()
}

// collectCodexRuleModels returns the request_models of every active rule under
// the Codex scenario, deduplicated and in declaration order.
func (aa *AgentApply) collectCodexRuleModels() []string {
	var models []string
	for _, rule := range aa.config.GetRequestConfigs() {
		if rule.GetScenario() != typ.ScenarioCodex || !rule.Active {
			continue
		}
		models = append(models, rule.RequestModel)
	}
	return CollectCodexModels(models)
}

// getBaseURLAndToken returns the base URL and API token for configuration
func (aa *AgentApply) getBaseURLAndToken() (string, string) {
	port := cmp.Or(aa.config.ServerPort, 12580)
	baseURL := fmt.Sprintf("http://%s:%d", aa.host, port)
	apiKey := aa.config.GetModelToken()
	return baseURL, apiKey
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
