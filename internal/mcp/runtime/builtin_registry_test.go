package runtime

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	mcptools "github.com/tingly-dev/tingly-box/internal/mcp/tools"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestRegisterBuiltinTools_OptionalSearchKey(t *testing.T) {
	for _, tc := range []struct {
		name       string
		processKey string
		configured string
		want       string
	}{
		{"no key", "", "", ""},
		{"legacy missing reference", "", "${SERPER_API_KEY}", ""},
		{"legacy dollar reference", "", "$SERPER_API_KEY", ""},
		{"inherited key", "system-test-key", "", "${SERPER_API_KEY}"},
		{"configured key", "system-test-key", "source-test-key", "source-test-key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SERPER_API_KEY", tc.processKey)
			if tc.processKey == "" {
				require.NoError(t, os.Unsetenv("SERPER_API_KEY"))
			}
			env := map[string]string{}
			if tc.configured != "" {
				env["SERPER_API_KEY"] = tc.configured
			}
			cfg := &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{{
				ID: mcptools.BuiltinWebtoolsSourceID, Enabled: typ.BoolPtr(true),
				Tools: []string{mcptools.BuiltinWebFetchToolName}, Env: env,
			}}}
			require.NoError(t, RegisterBuiltinTools(func() *typ.MCPRuntimeConfig { return cfg }, func(_ string, value interface{}) error {
				cfg = value.(*typ.MCPRuntimeConfig)
				return nil
			}))
			require.Equal(t, tc.want, cfg.Sources[0].Env["SERPER_API_KEY"])
			require.Empty(t, ValidateEnabledMCPSourceEnvRefs(cfg.Sources), "fetch must remain configurable without a search key")
		})
	}
}

func TestRegisterBuiltinTools_RestartPreservesAccessPolicies(t *testing.T) {
	t.Setenv("SERPER_API_KEY", "")
	profiles := []typ.MCPClientProfile{{ID: "reader", Sources: []string{"webtools"}, Tools: []string{"*"}}}
	cfg := &typ.MCPRuntimeConfig{ClientProfiles: profiles, ClientProfilesConfigured: true, Sources: []typ.MCPSourceConfig{
		{
			ID: mcptools.BuiltinWebtoolsSourceID, Name: "Public reader", Enabled: typ.BoolPtr(false),
			Usage: &typ.MCPToolUsage{Client: false, Gateway: true},
			ToolPolicies: map[string]typ.MCPToolPolicy{mcptools.BuiltinWebFetchToolName: {
				Enabled: typ.BoolPtr(false), Usage: &typ.MCPToolUsage{Client: true, Gateway: false},
			}},
		},
		{
			ID: mcptools.BuiltinAdvisorSourceID, Name: "Private reviewer", Enabled: typ.BoolPtr(true),
			Usage: &typ.MCPToolUsage{Client: false, Gateway: false},
			ToolPolicies: map[string]typ.MCPToolPolicy{mcptools.BuiltinAdvisorToolName: {
				Enabled: typ.BoolPtr(false), Usage: &typ.MCPToolUsage{Client: false, Gateway: true},
			}},
			Advisor: &typ.AdvisorConfig{ProviderUUID: "reviewer", Model: "review-model", TimeoutSeconds: 90},
		},
	}}
	webUsage, webPolicies := *cfg.Sources[0].Usage, cfg.Sources[0].ToolPolicies
	advisorUsage, advisorPolicies := *cfg.Sources[1].Usage, cfg.Sources[1].ToolPolicies
	for range 3 {
		require.NoError(t, RegisterBuiltinTools(func() *typ.MCPRuntimeConfig { return cfg }, func(_ string, value interface{}) error {
			cfg = value.(*typ.MCPRuntimeConfig)
			return nil
		}))
		require.False(t, *cfg.Sources[0].Enabled)
		require.Equal(t, "Public reader", cfg.Sources[0].Name)
		require.Equal(t, webUsage, *cfg.Sources[0].Usage)
		require.Equal(t, webPolicies, cfg.Sources[0].ToolPolicies)
		require.True(t, *cfg.Sources[1].Enabled)
		require.Equal(t, "Private reviewer", cfg.Sources[1].Name)
		require.Equal(t, advisorUsage, *cfg.Sources[1].Usage)
		require.Equal(t, advisorPolicies, cfg.Sources[1].ToolPolicies)
		require.Equal(t, "review-model", cfg.Sources[1].Advisor.Model)
		require.Equal(t, 90, cfg.Sources[1].Advisor.TimeoutSeconds)
		require.Equal(t, profiles, cfg.ClientProfiles)
		require.True(t, cfg.ClientProfilesConfigured)
	}
}

func TestRegisterBuiltinTools_RegistersWebtoolsAndAdvisor(t *testing.T) {
	var saved *typ.MCPRuntimeConfig
	err := RegisterBuiltinTools(
		func() *typ.MCPRuntimeConfig { return &typ.MCPRuntimeConfig{} },
		func(_ string, config interface{}) error {
			cfg, ok := config.(*typ.MCPRuntimeConfig)
			require.True(t, ok)
			saved = cfg
			return nil
		},
	)
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Len(t, saved.Sources, 2)

	var webtools, advisor *typ.MCPSourceConfig
	for i := range saved.Sources {
		switch saved.Sources[i].ID {
		case mcptools.BuiltinWebtoolsSourceID:
			webtools = &saved.Sources[i]
		case mcptools.BuiltinAdvisorSourceID:
			advisor = &saved.Sources[i]
		}
	}
	require.NotNil(t, webtools)
	require.NotNil(t, advisor)
	require.False(t, *advisor.Enabled)
	require.NotNil(t, advisor.Advisor)
	// No placeholder strings — fields are empty by default
	require.Empty(t, advisor.Advisor.ProviderUUID)
	require.Empty(t, advisor.Advisor.Model)
}

func TestRegisterBuiltinTools_PreservesExistingAdvisorSettings(t *testing.T) {
	enabled := typ.BoolPtr(true)
	advisorCfg := &typ.AdvisorConfig{
		ProviderUUID: "some-provider-uuid",
		Model:        "gpt-4.1",
	}

	input := &typ.MCPRuntimeConfig{
		Sources: []typ.MCPSourceConfig{
			{
				ID:      mcptools.BuiltinAdvisorSourceID,
				Enabled: enabled,
				Advisor: advisorCfg,
			},
		},
	}

	var saved *typ.MCPRuntimeConfig
	err := RegisterBuiltinTools(
		func() *typ.MCPRuntimeConfig { return input },
		func(_ string, config interface{}) error {
			cfg, ok := config.(*typ.MCPRuntimeConfig)
			require.True(t, ok)
			saved = cfg
			return nil
		},
	)
	require.NoError(t, err)
	require.NotNil(t, saved)

	var advisor *typ.MCPSourceConfig
	for i := range saved.Sources {
		if saved.Sources[i].ID == mcptools.BuiltinAdvisorSourceID {
			advisor = &saved.Sources[i]
			break
		}
	}
	require.NotNil(t, advisor)
	require.True(t, *advisor.Enabled)
	require.NotNil(t, advisor.Advisor)
	require.Equal(t, advisorCfg.ProviderUUID, advisor.Advisor.ProviderUUID)
	require.Equal(t, advisorCfg.Model, advisor.Advisor.Model)
}
