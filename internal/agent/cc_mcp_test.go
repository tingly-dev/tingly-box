package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	serverconfig "github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestMaterializeCCProfileMCPConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfg, err := serverconfig.NewConfig(serverconfig.WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cfg.CloseStores() })
	require.NoError(t, cfg.SetToolConfig(serverconfig.ToolTypeMCPRuntime, &typ.MCPRuntimeConfig{Profiles: []typ.MCPProfile{
		{ID: "prof-1a2b3c4d", Name: "Work Repo"},
		{ID: "prof-off", Name: "Off", Enabled: typ.BoolPtr(false)},
	}}))
	const base = "http://localhost:12580"
	profile := typ.ProfileMeta{ID: "p1", Name: "work"}
	settingsPath, err := CCProfileSettingsPath(profile.ID, profile.Name)
	require.NoError(t, err)
	file := filepath.Join(filepath.Dir(settingsPath), "mcp.json")

	// Nothing is connected until the user chooses an MCP profile.
	path, err := MaterializeCCProfileMCPConfig(cfg, base, profile)
	require.NoError(t, err)
	require.Empty(t, path)
	requirePathMissing(t, file)

	// A choice does nothing while MCP is off: the endpoint would refuse it.
	profile.MCPProfile = typ.MCPDefaultProfileID
	path, err = MaterializeCCProfileMCPConfig(cfg, base, profile)
	require.NoError(t, err)
	require.Empty(t, path)

	require.NoError(t, cfg.SetScenarioFlag(typ.ScenarioGlobal, constant.ExtensionMCP, true))
	path, err = MaterializeCCProfileMCPConfig(cfg, base, profile)
	require.NoError(t, err)
	require.Equal(t, file, path)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm(), "the file holds the MCP token")
	doc := readJSONMap(t, path)
	require.Equal(t, map[string]any{"mcpServers": map[string]any{"tb": map[string]any{
		"type":    "http",
		"url":     base + "/api/v1/mcp/default",
		"headers": map[string]any{"Authorization": "Bearer " + cfg.GetMCPToken()},
	}}}, doc, "an ordinary MCP server and nothing else: no permission rules")

	// A custom MCP profile is registered under the copy command's name.
	profile.MCPProfile = "prof-1a2b3c4d"
	_, err = MaterializeCCProfileMCPConfig(cfg, base, profile)
	require.NoError(t, err)
	servers := readJSONMap(t, file)["mcpServers"].(map[string]any)
	require.Contains(t, servers, "tb-work-repo")
	require.Equal(t, base+"/api/v1/mcp/prof-1a2b3c4d", servers["tb-work-repo"].(map[string]any)["url"])

	// A switched-off or missing MCP profile, or no choice, removes the file.
	for _, choice := range []string{"prof-off", "prof-gone", ""} {
		require.NoError(t, os.WriteFile(file, []byte("{}"), 0600))
		profile.MCPProfile = choice
		path, err = MaterializeCCProfileMCPConfig(cfg, base, profile)
		require.NoError(t, err)
		require.Empty(t, path, choice)
		requirePathMissing(t, file)
	}
}
