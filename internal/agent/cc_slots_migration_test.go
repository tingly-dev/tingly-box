package agent

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	serverconfig "github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ccModelKeys are the env vars the slot migration must leave untouched.
var ccModelKeys = []string{
	"ANTHROPIC_MODEL",
	"ANTHROPIC_DEFAULT_HAIKU_MODEL",
	"ANTHROPIC_DEFAULT_SONNET_MODEL",
	"ANTHROPIC_DEFAULT_OPUS_MODEL",
	"ANTHROPIC_DEFAULT_FABLE_MODEL",
	"CLAUDE_CODE_SUBAGENT_MODEL",
}

// oldStyleConfig writes a config as the unified/separate era left it, on
// disk and in the rule database, with the slot migration not yet run.
func oldStyleConfig(t *testing.T, separate bool) string {
	t.Helper()
	dir := t.TempDir()
	cfg, err := serverconfig.NewConfigWithDir(dir)
	require.NoError(t, err)

	svc := func(model string) []*loadbalance.Service {
		return []*loadbalance.Service{{Provider: "p", Model: model, Active: true}}
	}
	for i := range cfg.Rules {
		r := &cfg.Rules[i]
		if !r.GetScenario().Is(typ.ScenarioClaudeCode) {
			continue
		}
		if r.UUID == serverconfig.RuleUUIDCC {
			r.Active = !separate
			r.Services = svc("main")
		} else {
			r.Active = separate
		}
	}
	cfg.Rules = append(cfg.Rules, typ.Rule{
		UUID: serverconfig.RuleUUIDCCDefault, Scenario: typ.ScenarioClaudeCode,
		RequestModel: "tingly/cc-default", Active: separate, Services: svc("default-model"),
	})
	cfg.Scenarios = append(cfg.Scenarios, typ.ScenarioConfig{
		Scenario: typ.ScenarioClaudeCode,
		Flags:    typ.ScenarioFlags{Unified: !separate, Separate: separate},
	})
	cfg.MigrationsCompleted = slices.DeleteFunc(cfg.MigrationsCompleted, func(m string) bool { return m == "20261008-claude-code-slots" })
	require.NoError(t, cfg.Save())
	return dir
}

func TestClaudeCodeSlotMigration_EnvUnchangedAcrossRestart(t *testing.T) {
	for name, tc := range map[string]struct {
		separate bool
		want     map[string]string
	}{
		"unified": {false, map[string]string{
			"ANTHROPIC_MODEL": "tingly/cc", "ANTHROPIC_DEFAULT_HAIKU_MODEL": "tingly/cc",
			"ANTHROPIC_DEFAULT_SONNET_MODEL": "tingly/cc", "ANTHROPIC_DEFAULT_OPUS_MODEL": "tingly/cc",
			"ANTHROPIC_DEFAULT_FABLE_MODEL": "tingly/cc", "CLAUDE_CODE_SUBAGENT_MODEL": "tingly/cc",
		}},
		"separate": {true, map[string]string{
			"ANTHROPIC_MODEL": "tingly/cc-default", "ANTHROPIC_DEFAULT_HAIKU_MODEL": "tingly/cc-haiku",
			"ANTHROPIC_DEFAULT_SONNET_MODEL": "tingly/cc-sonnet", "ANTHROPIC_DEFAULT_OPUS_MODEL": "tingly/cc-opus",
			"ANTHROPIC_DEFAULT_FABLE_MODEL": "tingly/cc-fable", "CLAUDE_CODE_SUBAGENT_MODEL": "tingly/cc-subagent",
		}},
	} {
		t.Run(name, func(t *testing.T) {
			dir := oldStyleConfig(t, tc.separate)

			// Restart: hydrate rules from the database, run the migration.
			cfg, err := serverconfig.NewConfigWithDir(dir)
			require.NoError(t, err)
			env := GenerateCCEnv(cfg, "http://localhost:12580", "tok", "claude_code", false)
			for _, k := range ccModelKeys {
				require.Equal(t, tc.want[k], env[k], k)
			}
			require.Nil(t, cfg.GetRuleByUUID(serverconfig.RuleUUIDCCDefault))
			main := cfg.GetRuleByUUID(serverconfig.RuleUUIDCC)
			require.True(t, main.Active)
			wantSvc := "main"
			if tc.separate {
				wantSvc = "default-model"
			}
			require.Equal(t, wantSvc, main.Services[0].Model, "the main rule routes where ANTHROPIC_MODEL used to")

			// Again: the result was persisted and the migration does not rerun.
			again, err := serverconfig.NewConfigWithDir(dir)
			require.NoError(t, err)
			require.Equal(t, env, GenerateCCEnv(again, "http://localhost:12580", "tok", "claude_code", false))
			require.Nil(t, again.GetRuleByUUID(serverconfig.RuleUUIDCCDefault))
		})
	}
}
