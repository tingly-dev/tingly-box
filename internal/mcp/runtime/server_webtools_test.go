package runtime

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	mcptools "github.com/tingly-dev/tingly-box/internal/mcp/tools"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestServerWebtoolsRequireIndependentOptInAndStayOutOfProfiles(t *testing.T) {
	t.Setenv("SERPER_API_KEY", "")
	cfg := &typ.MCPRuntimeConfig{}
	save := func(_ string, value interface{}) error { cfg = value.(*typ.MCPRuntimeConfig); return nil }
	require.NoError(t, RegisterBuiltinTools(func() *typ.MCPRuntimeConfig { return cfg }, save))
	r := NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
	defer r.Close()
	var source *typ.MCPSourceConfig
	for i := range cfg.Sources {
		if cfg.Sources[i].ID == "server-webtools" {
			source = &cfg.Sources[i]
		}
	}
	require.NotNil(t, source)
	require.False(t, typ.IsMCPSourceEnabled(*source))
	status, err := r.DiscoverSource(context.Background(), source.ID)
	require.NoError(t, err)
	require.Equal(t, "disabled", status.State)
	require.Len(t, status.Tools, 2, "disabled tools must remain discoverable without launching stdio")
	require.Empty(t, r.ListBuiltinServerToolsForInjection(context.Background()))
	require.False(t, r.HasBuiltinServerTools())
	source.Enabled = typ.BoolPtr(true)
	require.False(t, r.HasBuiltinServerTools(), "source enablement alone must not enable its tools")
	source.ToolPolicies[mcptools.BuiltinWebFetchToolName] = typ.MCPToolPolicy{Enabled: typ.BoolPtr(true)}
	require.True(t, r.HasBuiltinServerTools())
	injected := r.ListBuiltinServerToolsForInjection(context.Background())
	require.Len(t, injected, 1)
	require.Equal(t, NormalizeToolName(source.ID, mcptools.BuiltinWebFetchToolName), injected[0].GetFunction().Name)
	require.Contains(t, r.ListCallableServerToolNames(context.Background()), injected[0].GetFunction().Name)
	require.False(t, SourceUsage(*source).Client)
	for _, profile := range r.Routing(context.Background()).Profiles {
		for _, route := range profile.Sources {
			require.NotEqual(t, source.ID, route.ID)
		}
	}
	source.ToolPolicies[mcptools.BuiltinWebSearchToolName] = typ.MCPToolPolicy{Enabled: typ.BoolPtr(true)}
	require.Len(t, r.ListBuiltinServerToolsForInjection(context.Background()), 1, "search alone needs a key")
	t.Setenv("SERPER_API_KEY", "process-key-is-not-configuration")
	require.False(t, r.IsBuiltinServerToolAvailable(source.ID, mcptools.BuiltinWebSearchToolName))
	_, callErr := r.CallTool(context.Background(), NormalizeToolName(source.ID, mcptools.BuiltinWebSearchToolName), `{}`)
	require.ErrorContains(t, callErr, "SERPER_API_KEY")
	status, err = r.DiscoverSource(context.Background(), source.ID)
	require.NoError(t, err)
	for _, tool := range status.Tools {
		require.True(t, tool.Enabled, "policy enablement must not conflate readiness")
	}
	source.Env["SERPER_API_KEY"] = "configured-key"
	require.Len(t, r.ListBuiltinServerToolsForInjection(context.Background()), 2)
	source.Env["SERPER_API_KEY"] = "${SERPER_API_KEY}"
	require.True(t, r.IsBuiltinServerToolAvailable(source.ID, mcptools.BuiltinWebSearchToolName))
	require.Equal(t, "${SERPER_API_KEY}", source.Env["SERPER_API_KEY"], "readiness must not mutate configured references")
	source.Env["SERPER_API_KEY"] = "configured-key"
	require.NoError(t, RegisterBuiltinTools(func() *typ.MCPRuntimeConfig { return cfg }, save))
	for _, s := range cfg.Sources {
		if s.ID == "server-webtools" {
			require.True(t, *s.ToolPolicies[mcptools.BuiltinWebFetchToolName].Enabled)
			require.Equal(t, "configured-key", s.Env["SERPER_API_KEY"])
		}
	}
}
