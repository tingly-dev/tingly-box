package tbclient

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"

	"github.com/sirupsen/logrus"
	tbagent "github.com/tingly-dev/tingly-box/internal/agent"
	serverconfig "github.com/tingly-dev/tingly-box/internal/config"
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
// through the local tingly-box gateway for the connection default (no named
// profile). The user's main Claude Code settings (~/.claude/settings.json)
// are the source of truth for every preference — output tokens, timeouts,
// proxies and any other env key — so they are layered over tb's defaults here
// and reach a default-routed thread, folder session and the coordinator. The
// rule-derived model slots and the gateway connection values are owned by tb
// and always win.
func (c *TBClientImpl) GetClaudeCodeEnv(ctx context.Context) ([]string, error) {
	port := c.config.GetServerPort()
	if port == 0 {
		port = 12580
	}
	host := c.config.GetServerHost()
	baseURL := fmt.Sprintf("http://%s:%d", host, port)
	apiKey := c.config.GetModelToken()

	// tb's canonical defaults seed a fresh setup (timeouts, output limit and
	// telemetry switches); an existing main settings file then overrides them,
	// key for key and without dropping any key it carries.
	envMap, err := tbagent.DefaultClaudeCodePrefs().Values()
	if err != nil {
		return nil, fmt.Errorf("build claude code env: %w", err)
	}
	if snapshot, readErr := tbagent.ReadMainClaudeCodeSettings(); readErr != nil {
		logrus.WithError(readErr).Warn("tbclient: cannot read the main Claude Code settings; using defaults")
	} else {
		maps.Copy(envMap, snapshot.Env)
	}

	models := c.resolveClaudeCodeModels()
	envMap["ANTHROPIC_MODEL"] = models.def
	envMap["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = models.haiku
	envMap["ANTHROPIC_DEFAULT_SONNET_MODEL"] = models.sonnet
	envMap["ANTHROPIC_DEFAULT_OPUS_MODEL"] = models.opus
	envMap["ANTHROPIC_DEFAULT_FABLE_MODEL"] = models.fable
	envMap["CLAUDE_CODE_SUBAGENT_MODEL"] = models.subagent

	// Connection values are owned by the current server context and always win.
	tbagent.ApplyConnectionEnv(envMap, baseURL, apiKey)
	envMap["TINGLY_API_URL"] = baseURL
	tbagent.OmitFollowedDefaultModel(envMap)

	env := make([]string, 0, len(envMap))
	for k, v := range envMap {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env, nil
}

// ClaudeCodeDefaultModel is the request model Claude Code is pointed at by
// default (ANTHROPIC_MODEL in GetClaudeCodeEnv): the claude_code scenario's
// default tier, which its routing rules resolve.
func (c *TBClientImpl) ClaudeCodeDefaultModel() string {
	return c.resolveClaudeCodeModels().def
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
	def, haiku, sonnet, opus, subagent, fable string
}

// resolveClaudeCodeModels resolves the per-tier request models of the main
// claude_code scenario through the same resolver as the settings file.
func (c *TBClientImpl) resolveClaudeCodeModels() claudeCodeModels {
	m, _ := tbagent.ClaudeCodeSlotModels(c.config, string(typ.ScenarioClaudeCode), false)
	return claudeCodeModels{
		def:      m["ANTHROPIC_MODEL"],
		haiku:    m["ANTHROPIC_DEFAULT_HAIKU_MODEL"],
		sonnet:   m["ANTHROPIC_DEFAULT_SONNET_MODEL"],
		opus:     m["ANTHROPIC_DEFAULT_OPUS_MODEL"],
		subagent: m["CLAUDE_CODE_SUBAGENT_MODEL"],
		fable:    m["ANTHROPIC_DEFAULT_FABLE_MODEL"],
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
	case typ.ScenarioWorkBuddy:
		return "/tingly/workbuddy"
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
