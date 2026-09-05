package config

import (
	"fmt"
	"maps"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ============
// Scenario Configuration
// ============

// GetScenarios returns all scenario configurations
func (c *Config) GetScenarios() []typ.ScenarioConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Scenarios == nil {
		return []typ.ScenarioConfig{}
	}
	return c.Scenarios
}

// GetScenarioConfig returns the configuration for a specific scenario
func (c *Config) GetScenarioConfig(scenario typ.RuleScenario) *typ.ScenarioConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.scenarioConfigLocked(scenario)
}

// scenarioConfigLocked returns the scenario config without acquiring the mutex.
// Callers must hold at least a read lock.
// For profiled scenarios (e.g., "claude_code:p1"):
// 1. First looks for profiled scenario's own config
// 2. Falls back to base scenario config if profiled config not found
func (c *Config) scenarioConfigLocked(scenario typ.RuleScenario) *typ.ScenarioConfig {
	// Try exact match first (for profiled scenarios with their own config)
	for i := range c.Scenarios {
		if c.Scenarios[i].Scenario == scenario {
			return &c.Scenarios[i]
		}
	}

	// For profiled scenarios, fallback to base scenario config
	baseScenario, profileID := typ.ParseScenarioProfile(scenario)
	if profileID != "" {
		for i := range c.Scenarios {
			if c.Scenarios[i].Scenario == baseScenario {
				return &c.Scenarios[i]
			}
		}
	}

	return nil
}

// SetScenarioConfig updates or creates a scenario configuration
func (c *Config) SetScenarioConfig(config typ.ScenarioConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if scenario already exists and update it
	for i := range c.Scenarios {
		if c.Scenarios[i].Scenario == config.Scenario {
			c.Scenarios[i] = config
			c.syncClaudeCodeRuleModeLocked(config)
			return c.Save()
		}
	}

	// Add new scenario config
	c.Scenarios = append(c.Scenarios, config)
	c.syncClaudeCodeRuleModeLocked(config)
	return c.Save()
}

func (c *Config) syncClaudeCodeRuleModeLocked(config typ.ScenarioConfig) {
	if config.Scenario != typ.ScenarioClaudeCode {
		return
	}

	flags := config.GetDefaultFlags()
	if flags.Separate {
		c.setClaudeCodeModeRulesActiveLocked(false, true)
		return
	}
	if flags.Unified {
		c.setClaudeCodeModeRulesActiveLocked(true, false)
	}
}

func (c *Config) setClaudeCodeModeRulesActiveLocked(unifiedActive, separateActive bool) {
	for i := range c.Rules {
		rule := &c.Rules[i]
		if !rule.GetScenario().Is(typ.ScenarioClaudeCode) {
			continue
		}
		if claudeCodeUnifiedRuleUUIDs[rule.UUID] {
			rule.Active = unifiedActive
			continue
		}
		if claudeCodeSeparateRuleUUIDs[rule.UUID] {
			rule.Active = separateActive
		}
	}
}

func (c *Config) GetScenarioFlag(scenario typ.RuleScenario, flagName string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	config := c.scenarioConfigLocked(scenario)
	if config == nil {
		return false
	}
	flags := config.GetDefaultFlags()
	switch flagName {
	case constant.FlagUnified:
		return flags.Unified
	case constant.FlagSeparate:
		return flags.Separate
	case constant.FlagSmartCompact:
		return flags.SmartCompact
	case constant.FlagSkipUsage:
		return flags.SkipUsage
	default:
		if config.Extensions == nil {
			return false
		}
		val, _ := config.Extensions[flagName].(bool)
		return val
	}
}

// findOrCreateScenarioConfigLocked returns the scenario config to mutate for a
// Set*Flag call. If an exact-match entry doesn't exist yet, seeds a new one:
// for a profiled scenario (e.g. "claude_code:p1") it copies Flags/Extensions
// from the resolved base config (via scenarioConfigLocked's fallback) so the
// profile starts as a fork of its base rather than silently losing flags that
// were previously inherited from the base scenario on first write. For a
// non-profiled (or otherwise unresolvable) scenario it creates a blank entry
// as before. Callers must hold the write lock.
func (c *Config) findOrCreateScenarioConfigLocked(scenario typ.RuleScenario) *typ.ScenarioConfig {
	for i := range c.Scenarios {
		if c.Scenarios[i].Scenario == scenario {
			return &c.Scenarios[i]
		}
	}

	newConfig := typ.ScenarioConfig{
		Scenario:   scenario,
		Flags:      typ.ScenarioFlags{},
		Extensions: make(map[string]interface{}),
	}
	if seed := c.scenarioConfigLocked(scenario); seed != nil {
		newConfig.Flags = seed.Flags
		newConfig.Extensions = make(map[string]interface{}, len(seed.Extensions))
		maps.Copy(newConfig.Extensions, seed.Extensions)
	}
	c.Scenarios = append(c.Scenarios, newConfig)
	return &c.Scenarios[len(c.Scenarios)-1]
}

// SetScenarioFlag sets a specific flag value for a scenario
func (c *Config) SetScenarioFlag(scenario typ.RuleScenario, flagName string, value bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Find or create scenario config (seeded from base config for profiles)
	config := c.findOrCreateScenarioConfigLocked(scenario)

	// Set the specific flag
	switch flagName {
	case constant.FlagUnified:
		config.Flags.Unified = value
		if scenario == typ.ScenarioClaudeCode && value {
			config.Flags.Separate = false
			c.setClaudeCodeModeRulesActiveLocked(true, false)
		}
	case constant.FlagSeparate:
		config.Flags.Separate = value
		if scenario == typ.ScenarioClaudeCode && value {
			config.Flags.Unified = false
			c.setClaudeCodeModeRulesActiveLocked(false, true)
		}
	case constant.FlagSmartCompact:
		config.Flags.SmartCompact = value
	case constant.FlagSkipUsage:
		config.Flags.SkipUsage = value
	case constant.ExtensionSkillUser:
		if config.Extensions == nil {
			config.Extensions = make(map[string]interface{})
		}
		config.Extensions[constant.ExtensionSkillUser] = value
	case constant.ExtensionSkillIDE:
		if config.Extensions == nil {
			config.Extensions = make(map[string]interface{})
		}
		config.Extensions[constant.ExtensionSkillIDE] = value
	case constant.ExtensionGuardrails:
		if config.Extensions == nil {
			config.Extensions = make(map[string]interface{})
		}
		config.Extensions[constant.ExtensionGuardrails] = value
	case constant.ExtensionMCP:
		if config.Extensions == nil {
			config.Extensions = make(map[string]interface{})
		}
		config.Extensions[constant.ExtensionMCP] = value
	default:
		return fmt.Errorf("unknown flag name: %s", flagName)
	}

	return c.Save()
}

// GetScenarioStringFlag returns a string flag value for a scenario
func (c *Config) GetScenarioStringFlag(scenario typ.RuleScenario, flagName string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	config := c.scenarioConfigLocked(scenario)
	if config == nil {
		return ""
	}
	flags := config.GetDefaultFlags()
	switch flagName {
	case constant.FlagThinkingEffort:
		return flags.ThinkingEffort
	case constant.FlagRecordingV2:
		return string(flags.RecordingV2)
	case constant.FlagCustomUserAgent:
		return flags.CustomUserAgent
	default:
		return ""
	}
}

// SetScenarioStringFlag sets a string flag value for a scenario
func (c *Config) SetScenarioStringFlag(scenario typ.RuleScenario, flagName string, value string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Find or create scenario config (seeded from base config for profiles)
	config := c.findOrCreateScenarioConfigLocked(scenario)

	// Set the specific flag
	switch flagName {
	case constant.FlagThinkingEffort:
		config.Flags.ThinkingEffort = typ.ThinkingEffortLevel(value)
	case constant.FlagRecordingV2:
		if !typ.IsValidRecordingMode(value) {
			return fmt.Errorf("invalid recording_v2 value: %s (must be empty, a comma-separated set of capture points client_request/upstream_request/upstream_response/client_response, or a legacy mode request/request_response/staged_request_response)", value)
		}
		// Store normalized: legacy enum values become point sets so the config
		// converges on the point-set form as flags are touched.
		config.Flags.RecordingV2 = typ.ParseRecordingMode(value)
	case constant.FlagCustomUserAgent:
		config.Flags.CustomUserAgent = value
	default:
		return fmt.Errorf("unknown string flag name: %s", flagName)
	}

	return c.Save()
}

// GetScenarioIntFlag returns an integer flag value for a scenario.
//
// Generic infra for scenario-level integer flags: the HTTP int-flag endpoint
// routes through here. No keys are currently registered (session_affinity moved
// to a rule-only flag), so it returns 0; adding a future scenario int flag is
// just a new case in the switch.
func (c *Config) GetScenarioIntFlag(scenario typ.RuleScenario, flagName string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	config := c.scenarioConfigLocked(scenario)
	if config == nil {
		return 0
	}
	switch flagName {
	// No scenario-level int flags currently registered; add cases here.
	default:
		return 0
	}
}

// SetScenarioIntFlag sets an integer flag value for a scenario.
//
// Generic infra (see GetScenarioIntFlag). With no keys registered every flag is
// rejected; adding a future scenario int flag is just a new case in the switch.
func (c *Config) SetScenarioIntFlag(scenario typ.RuleScenario, flagName string, value int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch flagName {
	// No scenario-level int flags currently registered; add cases here, e.g.
	//   case FlagFoo:
	//       config := c.findOrCreateScenarioConfigLocked(scenario)
	//       config.Flags.Foo = value
	//       return c.Save()
	default:
		return fmt.Errorf("unknown int flag name: %s", flagName)
	}
}

// GetScenarioExtensionBool returns a boolean value from scenario extensions.
func (c *Config) GetScenarioExtensionBool(scenario typ.RuleScenario, key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	config := c.GetScenarioConfig(scenario)
	if config == nil || config.Extensions == nil {
		return false
	}
	val, ok := config.Extensions[key].(bool)
	if !ok {
		return false
	}
	return val
}

// GetScenarioExtensionString returns a string value from scenario extensions.
func (c *Config) GetScenarioExtensionString(scenario typ.RuleScenario, key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	config := c.GetScenarioConfig(scenario)
	if config == nil || config.Extensions == nil {
		return ""
	}
	val, ok := config.Extensions[key].(string)
	if !ok {
		return ""
	}
	return val
}

// SetScenarioExtensions merges extension values into a scenario config.
func (c *Config) SetScenarioExtensions(scenario typ.RuleScenario, values map[string]interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Find or create scenario config (seeded from base config for profiles)
	config := c.findOrCreateScenarioConfigLocked(scenario)

	if config.Extensions == nil {
		config.Extensions = make(map[string]interface{})
	}
	for key, value := range values {
		if value == nil {
			delete(config.Extensions, key)
			continue
		}
		config.Extensions[key] = value
	}
	return c.Save()
}

// GetScenarioRecordingMode returns the effective recording mode for a scenario
// It checks both legacy Recording (bool) and new RecordV2 (RecordingMode)
// Priority: RecordV2 > legacy Recording
func (c *Config) GetScenarioRecordingMode(scenario typ.RuleScenario) typ.RecordingMode {
	c.mu.RLock()
	defer c.mu.RUnlock()

	config := c.GetScenarioConfig(scenario)
	if config == nil {
		return typ.RecordingModeDisabled
	}

	flags := config.GetDefaultFlags()

	if flags.RecordingV2 != typ.RecordingModeDisabled {
		return flags.RecordingV2
	}

	return typ.RecordingModeDisabled
}

// IsScenarioRecordingEnabled checks if recording is enabled for a scenario
func (c *Config) IsScenarioRecordingEnabled(scenario typ.RuleScenario) bool {
	return c.GetScenarioRecordingMode(scenario) != typ.RecordingModeDisabled
}
