package agent

import (
	"strings"

	serverconfig "github.com/tingly-dev/tingly-box/internal/config"
)

// ccDisplayTiers are the pinned model slots whose /model picker entry Claude
// Code lets us label: alias -> (model env key, label).
var ccDisplayTiers = []struct {
	alias, modelKey, label string
}{
	{"opus", "ANTHROPIC_DEFAULT_OPUS_MODEL", "Opus"},
	{"sonnet", "ANTHROPIC_DEFAULT_SONNET_MODEL", "Sonnet"},
	{"haiku", "ANTHROPIC_DEFAULT_HAIKU_MODEL", "Haiku"},
	{"fable", "ANTHROPIC_DEFAULT_FABLE_MODEL", "Fable"},
}

// ccDisplayEnvKeys lists the *_NAME / *_DESCRIPTION env keys derived from the
// model slots, so callers can treat them as rule-owned alongside the slots.
func ccDisplayEnvKeys() []string {
	keys := make([]string, 0, 2*len(ccDisplayTiers))
	for _, t := range ccDisplayTiers {
		keys = append(keys, t.modelKey+"_NAME", t.modelKey+"_DESCRIPTION")
	}
	return keys
}

// CCTierDisplayEnv derives the /model picker labels from the model slots in
// env: without them Claude Code shows the raw gateway rule name under a
// "Custom model" heading, which says nothing about which alias it backs.
// The label pairs the alias with the concrete rule it routes to, mirroring
// what the user configured. Slots that are unset yield no labels, and the
// "[1m]" marker is dropped (Claude Code shows context size separately).
func CCTierDisplayEnv(env map[string]string) map[string]string {
	out := map[string]string{}
	for _, t := range ccDisplayTiers {
		model := strings.TrimSuffix(strings.TrimSpace(env[t.modelKey]), serverconfig.Context1MSuffix)
		if model == "" {
			continue
		}
		out[t.modelKey+"_NAME"] = t.label + " · " + model
		out[t.modelKey+"_DESCRIPTION"] = "Routed by Tingly Box rule " + model
	}
	return out
}
