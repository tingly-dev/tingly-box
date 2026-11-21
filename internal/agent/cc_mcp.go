package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	serverconfig "github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ccProfileMCPConfigFile sits next to a profile's settings.json. Claude Code
// cannot read mcpServers from a --settings file, so the profile's MCP server
// travels in its own file, passed with --mcp-config.
const ccProfileMCPConfigFile = "mcp.json"

// MaterializeCCProfileMCPConfig writes the --mcp-config file that connects a
// Claude Code profile's launches to the MCP profile it names, and returns its
// path. It returns "" and removes a stale file when there is nothing to
// connect: the profile names no MCP profile (the user has not configured
// one), MCP is off, or the MCP profile is gone or switched off. The server is
// an ordinary MCP server for Claude Code: its own permissions decide each
// call, so no permission rule is written.
func MaterializeCCProfileMCPConfig(cfg *serverconfig.Config, baseURL string, profile typ.ProfileMeta) (string, error) {
	settingsPath, err := CCProfileSettingsPath(profile.ID, profile.Name)
	if err != nil {
		return "", err
	}
	path := filepath.Join(filepath.Dir(settingsPath), ccProfileMCPConfigFile)
	mcp, ok := ccProfileMCPProfile(cfg, profile.MCPProfile)
	if !ok {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("failed to remove stale MCP config: %w", err)
		}
		return "", nil
	}
	doc := map[string]any{"mcpServers": map[string]any{
		MCPServerName(mcp): map[string]any{
			"type":    "http",
			"url":     strings.TrimRight(baseURL, "/") + "/api/v1/mcp/" + mcp.ID,
			"headers": map[string]string{"Authorization": "Bearer " + cfg.GetMCPToken()},
		},
	}}
	content, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	if err := ensureCCProfileArtifactDir(filepath.Dir(path)); err != nil {
		return "", fmt.Errorf("failed to create profile directory: %w", err)
	}
	// It holds the MCP token: readable by the user only.
	if _, err := writeGeneratedFileIfChanged(path, append(content, '\n'), 0600); err != nil {
		return "", fmt.Errorf("failed to write MCP config: %w", err)
	}
	return path, nil
}

// ccProfileMCPProfile resolves the MCP profile a Claude Code profile names,
// when it can serve: MCP on, the profile present and not switched off.
func ccProfileMCPProfile(cfg *serverconfig.Config, id string) (typ.MCPProfile, bool) {
	if cfg == nil || id == "" {
		return typ.MCPProfile{}, false
	}
	if !cfg.GetScenarioFlag(typ.ScenarioGlobal, constant.ExtensionMCP) && !cfg.GetScenarioFlag(typ.ScenarioClaudeCode, constant.ExtensionMCP) {
		return typ.MCPProfile{}, false
	}
	var profiles []typ.MCPProfile
	if runtime := cfg.GetMCPRuntimeConfig(); runtime != nil {
		profiles = runtime.Profiles
	}
	for _, p := range profiles {
		if p.ID == id {
			return p, p.Enabled == nil || *p.Enabled
		}
	}
	// Default exists before anything about it is saved.
	if id == typ.MCPDefaultProfileID {
		return typ.MCPProfile{ID: id, Name: "Default"}, true
	}
	return typ.MCPProfile{}, false
}

var mcpServerNameRE = regexp.MustCompile(`[^a-z0-9]+`)

// MCPServerName is the name an MCP profile is registered under in an agent:
// "tb" for Default, "tb-<slug of the name>" otherwise. It matches the copy
// command on the MCP profile page (profileServerName), so a manual
// registration and a generated one are the same server, not two.
func MCPServerName(p typ.MCPProfile) string {
	if p.ID == typ.MCPDefaultProfileID {
		return "tb"
	}
	slug := strings.Trim(mcpServerNameRE.ReplaceAllString(strings.ToLower(p.Name), "-"), "-")
	if slug == "" {
		slug = p.ID
	}
	return "tb-" + slug
}
