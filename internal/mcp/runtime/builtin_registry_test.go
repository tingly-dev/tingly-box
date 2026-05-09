package runtime

import (
	"testing"

	"github.com/stretchr/testify/require"
	mcptools "github.com/tingly-dev/tingly-box/internal/mcp/tools"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

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
