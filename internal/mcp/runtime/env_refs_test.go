package runtime

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestExpandMCPRuntimeEnvRefs(t *testing.T) {
	cfg := &typ.MCPRuntimeConfig{
		Sources: []typ.MCPSourceConfig{
			{
				ID:        "advisor",
				Enabled:   typ.BoolPtr(true),
				Transport: "advisor",
				Env: map[string]string{
					"SOME_VAR": "${SOME_VAR}",
				},
				Advisor: &typ.AdvisorConfig{
					ProviderUUID: "abc-123",
					Model:        "gpt-4.1",
				},
			},
		},
	}

	// ProviderUUID and Model are plain strings — no expansion needed.
	issues := ExpandMCPRuntimeEnvRefs(cfg)
	require.Empty(t, issues) // SOME_VAR placeholder stays as-is (not in process env), but advisor fields are clean
	require.Equal(t, "abc-123", cfg.Sources[0].Advisor.ProviderUUID)
	require.Equal(t, "gpt-4.1", cfg.Sources[0].Advisor.Model)
}

func TestValidateEnabledMCPSourceEnvRefs(t *testing.T) {
	enabled := typ.BoolPtr(true)
	disabled := typ.BoolPtr(false)
	sources := []typ.MCPSourceConfig{
		{
			ID:        "advisor-enabled",
			Enabled:   enabled,
			Transport: "advisor",
			Env: map[string]string{
				"MISSING_KEY": "${MISSING_KEY}",
			},
			Advisor: &typ.AdvisorConfig{
				ProviderUUID: "uuid-123",
			},
		},
		{
			ID:        "advisor-disabled",
			Enabled:   disabled,
			Transport: "advisor",
			Advisor:   &typ.AdvisorConfig{},
		},
	}

	issues := ValidateEnabledMCPSourceEnvRefs(sources)
	// Only the enabled source's missing env var should be reported
	require.Len(t, issues, 1)
	require.Equal(t, "advisor-enabled", issues[0].SourceID)
	require.Equal(t, "MISSING_KEY", issues[0].VarName)
}

func TestValidateEnabledMCPSourceEnvRefs_UsesSourceEnv(t *testing.T) {
	enabled := typ.BoolPtr(true)
	sources := []typ.MCPSourceConfig{
		{
			ID:      "advisor-enabled",
			Enabled: enabled,
			Env: map[string]string{
				"MY_KEY": "resolved-value",
			},
			Advisor: &typ.AdvisorConfig{
				ProviderUUID: "uuid-123",
			},
		},
	}

	issues := ValidateEnabledMCPSourceEnvRefs(sources)
	require.Empty(t, issues)
}
