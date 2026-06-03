package tbclient

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	tbagent "github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// SmartGuideRuleUUID generates the rule UUID for a specific bot
// Uses the bot UUID directly as the rule UUID for consistency
func SmartGuideRuleUUID(botUUID string) string {
	return botUUID
}

// TBClient defines the interface for remote control interactions
type TBClient interface {

	// GetProviders returns all configured providers
	GetProviders(ctx context.Context) ([]ProviderInfo, error)

	GetDefaultRule(ctx context.Context) (*typ.Rule, error)

	// GetDefaultRuleForScenario returns the default rule for a specific scenario
	GetDefaultRuleForScenario(ctx context.Context, scenario typ.RuleScenario) (*typ.Rule, error)

	// GetDefaultService returns the default service configuration
	// This reuses the ClaudeCode scenario's active service
	// Returns base URL (ClaudeCode scenario), API key, provider, and model
	GetDefaultService(ctx context.Context) (*DefaultServiceConfig, error)

	// GetConnectionConfig returns base URL and API key
	// Base URL defaults to ClaudeCode scenario URL if not configured
	GetConnectionConfig(ctx context.Context) (*ConnectionConfig, error)

	// GetClaudeCodeEnv returns the environment variables (KEY=VALUE) that point
	// the Claude Code CLI at the tingly-box gateway's claude_code scenario, so
	// remote-control @cc sessions route through the configured provider
	// (including third-party model services) instead of the Anthropic API.
	GetClaudeCodeEnv(ctx context.Context) ([]string, error)

	// GetHTTPEndpointForScenario returns HTTP endpoint configuration for a scenario
	GetHTTPEndpointForScenario(ctx context.Context, scenario typ.RuleScenario) (*HTTPEndpointConfig, error)

	// EnsureSmartGuideRule ensures the _smart_guide rule exists and is configured correctly
	// Deprecated: Use EnsureSmartGuideRuleForBot for bot-specific rules
	EnsureSmartGuideRule(ctx context.Context, providerUUID, modelID string) error

	// EnsureSmartGuideRuleForBot ensures the _smart_guide rule exists for a specific bot
	// Each bot gets its own rule with UUID equal to botUUID
	EnsureSmartGuideRuleForBot(ctx context.Context, botUUID, botName, providerUUID, modelID string) error

	// DeleteSmartGuideRuleForBot removes the _smart_guide rule for a specific bot
	DeleteSmartGuideRuleForBot(ctx context.Context, botUUID string) error

	// SelectModel returns model configuration for @tb execution
	SelectModel(ctx context.Context, req ModelSelectionRequest) (*ModelConfig, error)

	// GetDataDir returns the data directory path for storing sessions and other data
	GetDataDir() string
}

// TBClientImpl implements TBClient interface
type TBClientImpl struct {
	config     *serverconfig.Config
	providerDB *db.ProviderStore
}

// NewTBClient creates a new TB client instance
func NewTBClient(
	cfg *serverconfig.Config,
	providerDB *db.ProviderStore,
) *TBClientImpl {
	return &TBClientImpl{
		config:     cfg,
		providerDB: providerDB,
	}
}

// GetProviders returns all configured providers
func (c *TBClientImpl) GetProviders(ctx context.Context) ([]ProviderInfo, error) {
	providers := c.config.ListProviders()
	result := make([]ProviderInfo, 0, len(providers))

	for _, p := range providers {
		result = append(result, ProviderInfo{
			UUID:     p.UUID,
			Name:     p.Name,
			APIBase:  p.APIBase,
			APIStyle: string(p.APIStyle),
			Enabled:  p.Enabled,
			Models:   p.Models, // Include cached models if available
		})
	}

	return result, nil
}

// buildBaseURL constructs the base URL from server config
func (c *TBClientImpl) buildBaseURL() string {
	port := c.config.GetServerPort()
	if port == 0 {
		port = 12580
	}
	return fmt.Sprintf("http://localhost:%d/tingly/claude_code", port)
}

// findFirstClaudeCodeRule finds the first active ClaudeCode rule
func (c *TBClientImpl) findFirstClaudeCodeRule() (*typ.Rule, error) {
	rules := c.config.GetRequestConfigs()
	for i, rule := range rules {
		if rule.GetScenario() == typ.ScenarioClaudeCode && rule.Active {
			return &rules[i], nil
		}
	}
	return nil, fmt.Errorf("no active ClaudeCode rules found")
}

// GetConnectionConfig returns base URL and API key
func (c *TBClientImpl) GetConnectionConfig(ctx context.Context) (*ConnectionConfig, error) {
	// For @tb, we use the ClaudeCode scenario URL as default
	// API key comes from the default or configured provider

	apiKey := c.config.GetModelToken()

	return &ConnectionConfig{
		BaseURL: c.buildBaseURL(),
		APIKey:  apiKey,
	}, nil
}

// GetClaudeCodeEnv builds the env vars needed to route the Claude Code CLI
// through the local tingly-box gateway. ANTHROPIC_BASE_URL points at the
// claude_code scenario endpoint and the per-tier ANTHROPIC_*_MODEL vars carry
// the request models the user's claude_code rules actually route. The model
// names are resolved exactly like the frontend's Claude Code config apply
// (derivePrefsFromRules): the `separate` scenario flag selects per-tier vs
// unified, and each model name is read from the matching rule's request_model
// (by stable UUID) so customized model identifiers route correctly.
func (c *TBClientImpl) GetClaudeCodeEnv(ctx context.Context) ([]string, error) {
	port := c.config.GetServerPort()
	if port == 0 {
		port = 12580
	}
	baseURL := fmt.Sprintf("http://localhost:%d", port)
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

// claudeCodeModels holds the request-model name for each Claude Code model tier.
type claudeCodeModels struct {
	def, haiku, sonnet, opus, subagent string
}

// resolveClaudeCodeModels resolves the per-tier request models the same way the
// frontend's derivePrefsFromRules does: the `separate` scenario flag selects
// per-tier vs unified naming, and each model is taken from the corresponding
// built-in rule's request_model (looked up by its stable UUID), falling back to
// tb's canonical model names when a rule is absent.
func (c *TBClientImpl) resolveClaudeCodeModels() claudeCodeModels {
	byUUID := map[string]string{}
	for _, rule := range c.config.GetRequestConfigs() {
		if rule.GetScenario() != typ.ScenarioClaudeCode || !rule.Active {
			continue
		}
		if m := strings.TrimSpace(rule.RequestModel); m != "" {
			byUUID[rule.UUID] = m
		}
	}

	ruleModel := func(uuid, fallback string) string {
		if m, ok := byUUID[uuid]; ok {
			return m
		}
		return fallback
	}

	// `separate` flag → per-tier models; anything else (unset/unified/smart)
	// → a single unified model for every tier.
	if sc := c.config.GetScenarioConfig(typ.ScenarioClaudeCode); sc != nil && sc.GetDefaultFlags().Separate {
		return claudeCodeModels{
			def:      ruleModel("built-in-cc-default", "tingly/cc-default"),
			haiku:    ruleModel("built-in-cc-haiku", "tingly/cc-haiku"),
			sonnet:   ruleModel("built-in-cc-sonnet", "tingly/cc-sonnet"),
			opus:     ruleModel("built-in-cc-opus", "tingly/cc-opus"),
			subagent: ruleModel("built-in-cc-subagent", "tingly/cc-subagent"),
		}
	}

	unified := ruleModel("built-in-cc", "tingly/cc")
	return claudeCodeModels{
		def:      unified,
		haiku:    unified,
		sonnet:   unified,
		opus:     unified,
		subagent: unified,
	}
}

func (c *TBClientImpl) GetDefaultRule(ctx context.Context) (*typ.Rule, error) {
	return c.findFirstClaudeCodeRule()
}

// GetDefaultService returns the default service configuration
// This reuses the ClaudeCode scenario's active service
func (c *TBClientImpl) GetDefaultService(ctx context.Context) (*DefaultServiceConfig, error) {
	firstRule, err := c.findFirstClaudeCodeRule()
	if err != nil {
		return nil, err
	}

	services := firstRule.GetServices()
	if len(services) == 0 {
		return nil, fmt.Errorf("no services configured in ClaudeCode rule")
	}

	firstService := services[0]
	provider, err := c.config.GetProviderByUUID(firstService.Provider)
	if err != nil || provider == nil {
		return nil, fmt.Errorf("provider not found for ClaudeCode rule: %s", firstService.Provider)
	}

	return &DefaultServiceConfig{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ModelID:      firstService.Model,
		BaseURL:      c.buildBaseURL(),
		APIKey:       provider.GetAccessToken(),
		APIStyle:     string(provider.APIStyle),
	}, nil
}

// SelectModel returns model configuration for @tb execution
func (c *TBClientImpl) SelectModel(ctx context.Context, req ModelSelectionRequest) (*ModelConfig, error) {
	var provider *typ.Provider

	if req.ProviderUUID == "" {
		return nil, fmt.Errorf("no provider found")
	}

	var err error
	provider, err = c.config.GetProviderByUUID(req.ProviderUUID)
	if err != nil || provider == nil {
		return nil, fmt.Errorf("provider not found: %s", req.ProviderUUID)
	}
	return &ModelConfig{
		ProviderUUID: provider.UUID,
		ModelID:      req.ModelID,
		BaseURL:      provider.APIBase,
		APIKey:       provider.GetAccessToken(),
		APIStyle:     string(provider.APIStyle),
	}, nil
}

// GetDataDir returns the data directory path for storing sessions and other data
func (c *TBClientImpl) GetDataDir() string {
	if c.config == nil {
		return ""
	}

	// Use ConfigDir as base, return data subdirectory
	configDir := c.config.ConfigDir
	if configDir == "" {
		// Fallback to default data directory
		return filepath.Join(".", "data")
	}

	return filepath.Join(configDir, "data")
}

// HTTPEndpointConfig represents HTTP endpoint configuration for a scenario
type HTTPEndpointConfig struct {
	BaseURL string // e.g., "http://localhost:12580/tingly/_smart_guide/"
	APIKey  string // tingly-box token
}

// GetDefaultRuleForScenario returns the default rule for a specific scenario
func (c *TBClientImpl) GetDefaultRuleForScenario(ctx context.Context, scenario typ.RuleScenario) (*typ.Rule, error) {
	rules := c.config.GetRequestConfigs()
	for i, rule := range rules {
		if rule.GetScenario() == scenario && rule.Active {
			return &rules[i], nil
		}
	}
	return nil, fmt.Errorf("no active rules found for scenario: %s", scenario)
}

// GetHTTPEndpointForScenario returns HTTP endpoint configuration for a scenario
func (c *TBClientImpl) GetHTTPEndpointForScenario(ctx context.Context, scenario typ.RuleScenario) (*HTTPEndpointConfig, error) {
	// Verify that a rule exists for this scenario (don't need to use it, just check existence)
	_, err := c.GetDefaultRuleForScenario(ctx, scenario)
	if err != nil {
		return nil, fmt.Errorf("failed to get rule for scenario %s: %w", scenario, err)
	}

	// Build the base URL for this scenario
	port := c.config.GetServerPort()
	if port == 0 {
		port = 12580
	}

	// Build scenario-specific path
	scenarioPath := c.GetScenarioEndpointPath(scenario)
	baseURL := fmt.Sprintf("http://localhost:%d%s", port, scenarioPath)

	// Get API key from config
	apiKey := c.config.GetModelToken()

	return &HTTPEndpointConfig{
		BaseURL: baseURL,
		APIKey:  apiKey,
	}, nil
}

// GetScenarioEndpointPath returns the endpoint path for a scenario
func (c *TBClientImpl) GetScenarioEndpointPath(scenario typ.RuleScenario) string {
	switch scenario {
	case typ.ScenarioSmartGuide:
		return "/tingly/_smart_guide"
	case typ.ScenarioClaudeCode:
		return "/tingly/claude_code"
	case typ.ScenarioOpenCode:
		return "/tingly/opencode"
	case typ.ScenarioXcode:
		return "/tingly/xcode"
	case typ.ScenarioVSCode:
		return "/tingly/vscode"
	default:
		// Default to OpenAI scenario path
		return "/tingly/openai"
	}
}

// EnsureSmartGuideRule ensures the _smart_guide rule exists and is configured correctly
// Deprecated: Use EnsureSmartGuideRuleForBot for bot-specific rules
func (c *TBClientImpl) EnsureSmartGuideRule(ctx context.Context, providerUUID, modelID string) error {
	return c.config.EnsureSmartGuideRule(providerUUID, modelID)
}

// EnsureSmartGuideRuleForBot ensures the _smart_guide rule exists for a specific bot
// Each bot gets its own rule with UUID: _internal_smart_guide_{botUUID}
func (c *TBClientImpl) EnsureSmartGuideRuleForBot(ctx context.Context, botUUID, botName, providerUUID, modelID string) error {
	return c.config.EnsureSmartGuideRuleForBot(botUUID, botName, providerUUID, modelID)
}

// DeleteSmartGuideRuleForBot removes the _smart_guide rule for a specific bot
func (c *TBClientImpl) DeleteSmartGuideRuleForBot(ctx context.Context, botUUID string) error {
	ruleUUID := SmartGuideRuleUUID(botUUID)
	return c.config.DeleteRule(ruleUUID)
}
