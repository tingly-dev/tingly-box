package runtime

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestExpandMCPRuntimeEnvRefs(t *testing.T) {
	t.Setenv("ADVISOR_API_KEY", "sk-test")
	t.Setenv("ADVISOR_MODEL", "claude-opus-4-6")

	cfg := &typ.MCPRuntimeConfig{
		Sources: []typ.MCPSourceConfig{
			{
				ID:        "advisor",
				Enabled:   typ.BoolPtr(true),
				Transport: "advisor",
				Env: map[string]string{
					"ADVISOR_API_KEY": "${ADVISOR_API_KEY}",
					"ADVISOR_MODEL":   "${ADVISOR_MODEL}",
				},
				Advisor: &typ.AdvisorConfig{
					APIKey: "${ADVISOR_API_KEY}",
					Model:  "${ADVISOR_MODEL}",
				},
			},
		},
	}

	issues := ExpandMCPRuntimeEnvRefs(cfg)
	require.Empty(t, issues)
	require.Equal(t, "sk-test", cfg.Sources[0].Advisor.APIKey)
	require.Equal(t, "claude-opus-4-6", cfg.Sources[0].Advisor.Model)
}

func TestValidateEnabledMCPSourceEnvRefs(t *testing.T) {
	enabled := typ.BoolPtr(true)
	disabled := typ.BoolPtr(false)
	sources := []typ.MCPSourceConfig{
		{
			ID:        "advisor-enabled",
			Enabled:   enabled,
			Transport: "advisor",
			Advisor: &typ.AdvisorConfig{
				APIKey: "${MISSING_ADVISOR_KEY}",
			},
		},
		{
			ID:        "advisor-disabled",
			Enabled:   disabled,
			Transport: "advisor",
			Advisor: &typ.AdvisorConfig{
				APIKey: "${MISSING_DISABLED_KEY}",
			},
		},
	}

	issues := ValidateEnabledMCPSourceEnvRefs(sources)
	require.Len(t, issues, 1)
	require.Equal(t, "advisor-enabled", issues[0].SourceID)
	require.Equal(t, "MISSING_ADVISOR_KEY", issues[0].VarName)
	require.Contains(t, issues[0].FieldPath, "advisor.api_key")
}

func TestValidateEnabledMCPSourceEnvRefs_UsesSourceEnv(t *testing.T) {
	enabled := typ.BoolPtr(true)
	sources := []typ.MCPSourceConfig{
		{
			ID:        "advisor-enabled",
			Enabled:   enabled,
			Transport: "advisor",
			Env: map[string]string{
				"ADVISOR_API_KEY": "sk-from-source-env",
			},
			Advisor: &typ.AdvisorConfig{
				APIKey: "${ADVISOR_API_KEY}",
			},
		},
	}

	issues := ValidateEnabledMCPSourceEnvRefs(sources)
	require.Empty(t, issues)
}

func TestValidateEnabledMCPSourceEnvRefs_DoesNotImplicitlyUseProcessEnv(t *testing.T) {
	t.Setenv("ADVISOR_API_KEY", "sk-from-process")
	enabled := typ.BoolPtr(true)
	sources := []typ.MCPSourceConfig{
		{
			ID:        "advisor-enabled",
			Enabled:   enabled,
			Transport: "advisor",
			Advisor: &typ.AdvisorConfig{
				APIKey: "${ADVISOR_API_KEY}",
			},
		},
	}

	issues := ValidateEnabledMCPSourceEnvRefs(sources)
	require.Len(t, issues, 1)
	require.Equal(t, "ADVISOR_API_KEY", issues[0].VarName)
}
