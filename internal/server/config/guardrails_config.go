package config

import "github.com/tingly-dev/tingly-box/internal/typ"

// applyGuardrailsDefaults ensures the global scenario has an extensions map so
// guardrails can be toggled explicitly later. Guardrails themselves stay opt-in:
// a missing flag should behave as disabled rather than forcing runtime startup
// before a guardrails config exists.
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

	return updated
}
