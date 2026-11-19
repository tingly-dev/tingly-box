package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	serverconfig "github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// `agent apply codex` (CLI / TUI) and the Agent page's apply must write the
// same catalog: the page reads the file back and compares it with what the
// rules call for, so a CLI apply that drops the per-rule 1M window leaves a
// diff that nothing but the page's own apply can clear.
func TestApplyAgentCodex_ReadsBackAsApplied(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfg, err := serverconfig.NewConfig(serverconfig.WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	oneM := typ.Rule{UUID: "b", Scenario: typ.ScenarioCodex, RequestModel: "gpt-5.6-terra", Active: true}
	oneM.Flags.Context1M = true
	cfg.Rules = []typ.Rule{
		{UUID: "a", Scenario: typ.ScenarioCodex, RequestModel: "gpt-5.6-luna", Active: true},
		oneM,
	}

	_, err = NewAgentApply(cfg, "localhost").ApplyAgent(&ApplyAgentRequest{AgentType: AgentTypeCodex, Yes: true})
	require.NoError(t, err)

	applied, err := serverconfig.ReadCodexAppliedState()
	require.NoError(t, err)
	state, diffs := serverconfig.CompareCodexConfig(applied,
		[]string{"gpt-5.6-luna", "gpt-5.6-terra"}, serverconfig.BuildContextWindowsFromRules(cfg))
	assert.Equal(t, serverconfig.ClientConfigApplied, state, "%+v", diffs)
}
