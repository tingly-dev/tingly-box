package agent

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	serverconfig "github.com/tingly-dev/tingly-box/internal/config"
)

// `tb agent apply claude-code --provider … --model …` turns the main rule on
// before writing settings.json, so the first apply already advertises it as
// that rule says: here with the [1m] suffix of its 1M flag, which an off rule
// doesn't get.
func TestApplyAgent_ClaudeCodeWritesSlotModelsOfTheRuleBeingSet(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfg, provider := newApplyTestConfig(t)
	main := cfg.GetRuleByUUID(serverconfig.RuleUUIDCC)
	main.Active = false
	main.Flags.Context1M = true
	require.NoError(t, cfg.UpdateRule(main.UUID, *main))
	require.Equal(t, "cc", BuildClaudeCodeModelConfig(cfg).Default)

	result, err := NewAgentApply(cfg, "localhost").ApplyAgent(&ApplyAgentRequest{AgentType: AgentTypeClaudeCode, Provider: provider, Model: "big"})
	require.NoError(t, err)
	require.True(t, result.Success, result.Message)
	assert.True(t, cfg.GetRuleByUUID(serverconfig.RuleUUIDCC).Active)

	env, err := ReadClaudeCodeSettingsEnv(filepath.Join(home, ".claude", "settings.json"))
	require.NoError(t, err)
	for _, key := range []string{"ANTHROPIC_DEFAULT_SONNET_MODEL", "ANTHROPIC_DEFAULT_OPUS_MODEL", "CLAUDE_CODE_SUBAGENT_MODEL"} {
		assert.Equal(t, "cc"+serverconfig.Context1MSuffix, env[key], key)
	}
}
