package agent

import (
	"strings"

	serverconfig "github.com/tingly-dev/tingly-box/internal/config"
)

// ccDisplayEnvKeys lists the *_NAME / *_DESCRIPTION env keys derived from the
// pinned model slots, so callers can treat them as rule-owned alongside the
// slots. The slots are the tier aliases in cc_tiers.go.
func ccDisplayEnvKeys() []string {
	keys := make([]string, 0, 2*len(ClaudeCodeTierAliases))
	for _, alias := range ClaudeCodeTierAliases {
		slot := claudeCodeTierEnvKeys[alias]
		keys = append(keys, slot+"_NAME", slot+"_DESCRIPTION")
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
	for _, alias := range ClaudeCodeTierAliases {
		slot := claudeCodeTierEnvKeys[alias]
		model := strings.TrimSuffix(strings.TrimSpace(env[slot]), serverconfig.Context1MSuffix)
		if model == "" {
			continue
		}
		label := strings.ToUpper(alias[:1]) + alias[1:]
		if model == alias {
			// New installs name the rule after the alias; repeating it adds nothing.
			out[slot+"_NAME"] = label + " · Tingly Box"
			out[slot+"_DESCRIPTION"] = "Routed by Tingly Box"
			continue
		}
		out[slot+"_NAME"] = label + " · " + model
		out[slot+"_DESCRIPTION"] = "Routed by Tingly Box rule " + model
	}
	return out
}
