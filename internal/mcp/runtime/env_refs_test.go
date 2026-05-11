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
	// SOME_VAR is unresolved so it may appear as an issue, but advisor fields are clean.
	_ = ExpandMCPRuntimeEnvRefs(cfg)
	require.Equal(t, "abc-123", cfg.Sources[0].Advisor.ProviderUUID)
	require.Equal(t, "gpt-4.1", cfg.Sources[0].Advisor.Model)
}

func TestValidateEnabledMCPSourceEnvRefs(t *testing.T) {
	enabled := typ.BoolPtr(true)
	disabled := typ.BoolPtr(false)
	sources := []typ.MCPSourceConfig{
		{
			// Non-advisor transport — env refs are validated.
			ID:        "custom-enabled",
			Enabled:   enabled,
			Transport: "stdio",
			Env: map[string]string{
				"MISSING_KEY": "${MISSING_KEY}",
			},
		},
		{
			ID:      "custom-disabled",
			Enabled: disabled,
			Env: map[string]string{
				"MISSING_KEY": "${MISSING_KEY}",
			},
		},
		{
			// Advisor transport — env refs are skipped regardless of enabled state.
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
	}

	issues := ValidateEnabledMCPSourceEnvRefs(sources)
	// Only the enabled stdio source's missing env var should be reported.
	// Disabled source and advisor transport source are both skipped.
	require.NotEmpty(t, issues)
	for _, issue := range issues {
		require.Equal(t, "custom-enabled", issue.SourceID)
		require.Equal(t, "MISSING_KEY", issue.VarName)
	}
}

func TestValidateEnabledMCPSourceEnvRefs_UsesSourceEnv(t *testing.T) {
	enabled := typ.BoolPtr(true)
	sources := []typ.MCPSourceConfig{
		{
			ID:        "custom-enabled",
			Enabled:   enabled,
			Transport: "stdio",
			Env: map[string]string{
				"MY_KEY": "resolved-value",
			},
		},
	}

	issues := ValidateEnabledMCPSourceEnvRefs(sources)
	require.Empty(t, issues)
}
