package config

import "github.com/tingly-dev/tingly-box/internal/typ"

// applyGuardrailsDefaults enables guardrails by default for testing when not set.
func (c *Config) applyGuardrailsDefaults() bool {
	updated := false
	cfg := c.GetScenarioConfig(typ.ScenarioGlobal)
	if cfg == nil {
		c.Scenarios = append(c.Scenarios, typ.ScenarioConfig{
			Scenario:   typ.ScenarioGlobal,
			Extensions: map[string]interface{}{},
		})
		cfg = &c.Scenarios[len(c.Scenarios)-1]
		updated = true
	}

	if cfg.Extensions == nil {
		cfg.Extensions = make(map[string]interface{})
		updated = true
	}

	if _, exists := cfg.Extensions["guardrails"]; !exists {
		cfg.Extensions["guardrails"] = true
		updated = true
	}

	return updated
}
