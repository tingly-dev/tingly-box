package tbclient

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	tbagent "github.com/tingly-dev/tingly-box/internal/agent"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TBClient defines the interface for remote control interactions with the
// tingly-box server. Both an in-memory implementation (TBClientImpl, for
// in-process use) and an HTTP implementation (HTTPTBClient, for
// out-of-process use) satisfy this interface.
type TBClient interface {
	// GetClaudeCodeEnv returns the environment variables (KEY=VALUE) that point
	// the Claude Code CLI at the tingly-box gateway's claude_code scenario.
	GetClaudeCodeEnv(ctx context.Context) ([]string, error)

	// GetClaudeCodeSettingsPathForProfile materializes a Claude Code profile's
	// settings.json and returns its path for the --settings CLI flag.
	// Returns ("", nil) for an empty profileID.
	GetClaudeCodeSettingsPathForProfile(ctx context.Context, profileID string) (string, error)

	// GetHTTPEndpointForScenario returns HTTP endpoint configuration for a scenario.
	GetHTTPEndpointForScenario(ctx context.Context, scenario typ.RuleScenario) (*HTTPEndpointConfig, error)

	// EnsureSmartGuideRuleForBot ensures the _smart_guide rule exists for a specific bot.
	EnsureSmartGuideRuleForBot(ctx context.Context, botUUID, botName, providerUUID, modelID string) error

	// DeleteSmartGuideRuleForBot removes the _smart_guide rule for a specific bot.
	DeleteSmartGuideRuleForBot(ctx context.Context, botUUID string) error

	// GetDataDir returns the data directory path for storing sessions and other data.
	GetDataDir() string
}

// HTTPEndpointConfig represents HTTP endpoint configuration for a scenario.
type HTTPEndpointConfig struct {
	BaseURL string
	APIKey  string
}

// ---------------------------------------------------------------------------
// TBClientImpl — in-memory implementation (same-process)
// ---------------------------------------------------------------------------

// TBClientImpl implements TBClient via direct in-memory config access.
// Use this when the consumer runs in the same process as the server.
type TBClientImpl struct {
	config *serverconfig.Config
}

// NewTBClient creates a new in-memory TB client instance.
func NewTBClient(cfg *serverconfig.Config) *TBClientImpl {
	return &TBClientImpl{config: cfg}
}

// GetClaudeCodeEnv builds the env vars needed to route the Claude Code CLI
// through the local tingly-box gateway.
func (c *TBClientImpl) GetClaudeCodeEnv(ctx context.Context) ([]string, error) {
	port := c.config.GetServerPort()
	if port == 0 {
		port = 12580
	}
	host := c.config.GetServerHost()
	baseURL := fmt.Sprintf("http://%s:%d", host, port)
	apiKey := c.config.GetModelToken()

	models := c.resolveClaudeCodeModels()
	prefs := tbagent.DefaultClaudeCodePrefs(false)
	prefs.AnthropicModel = models.def
	prefs.AnthropicDefaultHaikuModel = models.haiku
	prefs.AnthropicDefaultSonnetModel = models.sonnet
	prefs.AnthropicDefaultOpusModel = models.opus
	prefs.ClaudeCodeSubagentModel = models.subagent

	envMap, err := prefs.ToEnv(baseURL, apiKey)
	if err != nil {
		return nil, fmt.Errorf("build claude code env: %w", err)
	}

	env := make([]string, 0, len(envMap))
	for k, v := range envMap {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env, nil
}

// GetClaudeCodeSettingsPathForProfile materializes the profiled scenario
// settings file and returns its path for the --settings CLI flag.
func (c *TBClientImpl) GetClaudeCodeSettingsPathForProfile(ctx context.Context, profileID string) (string, error) {
	if profileID == "" {
		return "", nil
	}

	meta, found := c.config.GetProfile(typ.ScenarioClaudeCode, profileID)
	if !found {
		return "", fmt.Errorf("claude code profile %q not found", profileID)
	}

	port := c.config.GetServerPort()
	if port == 0 {
		port = 12580
	}
	host := c.config.GetServerHost()
	baseURL := fmt.Sprintf("http://%s:%d", host, port)
	apiKey := c.config.GetModelToken()
	scenarioPath := string(typ.ProfiledScenarioName(typ.ScenarioClaudeCode, meta.ID))

	settingsPath, err := tbagent.MaterializeCCProfileSettings(c.config, baseURL, apiKey, scenarioPath, meta)
	if err != nil {
		return "", fmt.Errorf("materialize claude code profile %q settings: %w", profileID, err)
	}
	return settingsPath, nil
}

// GetHTTPEndpointForScenario returns HTTP endpoint configuration for a scenario.
func (c *TBClientImpl) GetHTTPEndpointForScenario(ctx context.Context, scenario typ.RuleScenario) (*HTTPEndpointConfig, error) {
	if _, err := c.findFirstRuleForScenario(scenario); err != nil {
		return nil, fmt.Errorf("failed to get rule for scenario %s: %w", scenario, err)
	}

	port := c.config.GetServerPort()
	if port == 0 {
		port = 12580
	}
	host := c.config.GetServerHost()
	scenarioPath := GetScenarioEndpointPath(scenario)
	baseURL := fmt.Sprintf("http://%s:%d%s", host, port, scenarioPath)
	apiKey := c.config.GetModelToken()

	return &HTTPEndpointConfig{
		BaseURL: baseURL,
		APIKey:  apiKey,
	}, nil
}

// EnsureSmartGuideRuleForBot ensures the _smart_guide rule exists for a specific bot.
func (c *TBClientImpl) EnsureSmartGuideRuleForBot(ctx context.Context, botUUID, botName, providerUUID, modelID string) error {
	return c.config.EnsureSmartGuideRuleForBot(botUUID, botName, providerUUID, modelID)
}

// DeleteSmartGuideRuleForBot removes the _smart_guide rule for a specific bot.
func (c *TBClientImpl) DeleteSmartGuideRuleForBot(ctx context.Context, botUUID string) error {
	ruleUUID := serverconfig.SmartGuideRuleUUID(botUUID)
	return c.config.DeleteRule(ruleUUID)
}

// GetDataDir returns the data directory path for storing sessions and other data.
func (c *TBClientImpl) GetDataDir() string {
	if c.config == nil {
		return ""
	}
	configDir := c.config.ConfigDir
	if configDir == "" {
		return filepath.Join(".", "data")
	}
	return filepath.Join(configDir, "data")
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// claudeCodeModels holds the request-model name for each Claude Code model tier.
type claudeCodeModels struct {
	def, haiku, sonnet, opus, subagent string
}

// resolveClaudeCodeModels resolves the per-tier request models the same way the
// frontend's derivePrefsFromRules does.
func (c *TBClientImpl) resolveClaudeCodeModels() claudeCodeModels {
	byUUID := map[string]string{}
	for _, rule := range c.config.GetRequestConfigs() {
		if rule.GetScenario() != typ.ScenarioClaudeCode || !rule.Active {
			continue
		}
		if m := strings.TrimSpace(rule.RequestModel); m != "" {
			if rule.Flags.Context1M && !strings.HasSuffix(m, serverconfig.Context1MSuffix) {
				m += serverconfig.Context1MSuffix
			}
			byUUID[rule.UUID] = m
		}
	}

	ruleModel := func(uuid, legacyUUID, fallback string) string {
		if m, ok := byUUID[uuid]; ok {
			return m
		}
		if m, ok := byUUID[legacyUUID]; ok {
			return m
		}
		return fallback
	}

	if sc := c.config.GetScenarioConfig(typ.ScenarioClaudeCode); sc != nil && sc.GetDefaultFlags().Separate {
		return claudeCodeModels{
			def:      ruleModel("builtin:claude_code:default", "built-in-cc-default", "tingly/cc-default"),
			haiku:    ruleModel("builtin:claude_code:haiku", "built-in-cc-haiku", "tingly/cc-haiku"),
			sonnet:   ruleModel("builtin:claude_code:sonnet", "built-in-cc-sonnet", "tingly/cc-sonnet"),
			opus:     ruleModel("builtin:claude_code:opus", "built-in-cc-opus", "tingly/cc-opus"),
			subagent: ruleModel("builtin:claude_code:subagent", "built-in-cc-subagent", "tingly/cc-subagent"),
		}
	}

	unified := ruleModel("builtin:claude_code:cc", "built-in-cc", "tingly/cc")
	return claudeCodeModels{
		def:      unified,
		haiku:    unified,
		sonnet:   unified,
		opus:     unified,
		subagent: unified,
	}
}

// findFirstRuleForScenario finds the first active rule for the given scenario.
func (c *TBClientImpl) findFirstRuleForScenario(scenario typ.RuleScenario) (*typ.Rule, error) {
	rules := c.config.GetRequestConfigs()
	for i, rule := range rules {
		if rule.GetScenario() == scenario && rule.Active {
			return &rules[i], nil
		}
	}
	return nil, fmt.Errorf("no active rules found for scenario: %s", scenario)
}

// GetScenarioEndpointPath returns the endpoint path for a scenario.
// Exported because the HTTP client also needs it.
func GetScenarioEndpointPath(scenario typ.RuleScenario) string {
	switch scenario.Base() {
	case typ.ScenarioSmartGuide:
		return "/tingly/_smart_guide"
	case typ.ScenarioClaudeCode:
		return "/tingly/claude_code"
	case typ.ScenarioOpenCode:
		return "/tingly/opencode"
	case typ.ScenarioPi:
		return "/tingly/pi"
	case typ.ScenarioDsh:
		return "/tingly/dsh"
	case typ.ScenarioXcode:
		return "/tingly/xcode"
	case typ.ScenarioVSCode:
		return "/tingly/vscode"
	case typ.ScenarioCursor:
		return "/tingly/cursor"
	case typ.ScenarioTeam:
		return "/tingly/team"
	case typ.ScenarioCustom:
		return "/tingly/custom"
	default:
		return "/tingly/openai"
	}
}
