package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	serverconfig "github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestSplitSubagentSlot(t *testing.T) {
	cfg, err := serverconfig.NewConfigWithDir(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, cfg.AddProvider(&typ.Provider{
		UUID: "p", Name: "p", APIBase: "https://api.example.com",
		APIStyle: protocol.APIStyleAnthropic, AuthType: typ.AuthTypeAPIKey, Token: "t", Enabled: true,
	}))

	require.NoError(t, splitSubagentSlot(cfg, "p", "small-model"))

	assert.Equal(t, []string{"subagent"}, cfg.ClaudeCodeSlots(typ.ScenarioClaudeCode))
	rule := cfg.GetRuleByUUID(serverconfig.RuleUUIDCCSubagent)
	require.Len(t, rule.Services, 1)
	assert.Equal(t, "small-model", rule.Services[0].Model)
	assert.True(t, rule.Active)
}
