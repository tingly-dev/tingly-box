package command

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestProfileSlotsLabel(t *testing.T) {
	cfg, err := config.NewConfigWithDir(t.TempDir())
	require.NoError(t, err)
	solo, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "solo")
	require.NoError(t, err)
	require.Equal(t, "default", profileSlotsLabel(cfg, solo))

	scenario := typ.ProfiledScenarioName(typ.ScenarioClaudeCode, solo.ID)
	for _, slot := range []string{"haiku", "subagent"} {
		_, err = cfg.SetClaudeCodeSlot(scenario, slot, true)
		require.NoError(t, err)
	}
	require.Equal(t, "default + haiku, subagent", profileSlotsLabel(cfg, solo))
}
