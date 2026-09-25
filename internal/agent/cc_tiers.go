package agent

import (
	"fmt"
	"strings"

	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
)

// ClaudeCodeTierAliases are the --model aliases Claude Code maps to a tier
// env var. The default tier ("", no --model) is ANTHROPIC_MODEL.
var ClaudeCodeTierAliases = []string{"opus", "sonnet", "haiku"}

var claudeCodeTierEnvKeys = map[string]string{
	"":       "ANTHROPIC_MODEL",
	"opus":   "ANTHROPIC_DEFAULT_OPUS_MODEL",
	"sonnet": "ANTHROPIC_DEFAULT_SONNET_MODEL",
	"haiku":  "ANTHROPIC_DEFAULT_HAIKU_MODEL",
}

// ClaudeCodeTier is one model Claude Code can be asked for: the alias passed
// as --model ("" for the default) and the gateway model id it requests.
type ClaudeCodeTier struct {
	Alias string
	Model string
}

// ClaudeCodeTiers is what a Claude Code env offers. Unified means every
// tier requests the same model; Tiers then holds just the default.
type ClaudeCodeTiers struct {
	Unified bool
	Tiers   []ClaudeCodeTier
}

// ClaudeCodeTiersFromEnv reads the tiers from the env Claude Code is given
// (GenerateCCEnv's output, the main env or a profile's settings file), so
// they are exactly what the process will request. The "[1m]" marker is
// dropped: Claude Code strips it before sending, so the gateway never sees it.
func ClaudeCodeTiersFromEnv(env map[string]string) ClaudeCodeTiers {
	model := func(alias string) string {
		return strings.TrimSuffix(env[claudeCodeTierEnvKeys[alias]], serverconfig.Context1MSuffix)
	}
	def := model("")
	out := ClaudeCodeTiers{Unified: true, Tiers: []ClaudeCodeTier{{Alias: "", Model: def}}}
	for _, alias := range ClaudeCodeTierAliases {
		m := model(alias)
		if m == "" {
			continue
		}
		if m != def {
			out.Unified = false
		}
		out.Tiers = append(out.Tiers, ClaudeCodeTier{Alias: alias, Model: m})
	}
	if out.Unified {
		out.Tiers = out.Tiers[:1]
	}
	return out
}

// ReadClaudeCodeSettingsEnv returns the env block of a Claude Code settings
// file (a profile's materialized settings.json).
func ReadClaudeCodeSettingsEnv(path string) (map[string]string, error) {
	snapshot, err := readClaudeCodeSettings(path)
	if err != nil {
		return nil, err
	}
	if !snapshot.Exists {
		return nil, fmt.Errorf("claude code settings %s not found", path)
	}
	return snapshot.Env, nil
}
