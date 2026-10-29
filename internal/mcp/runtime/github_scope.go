package runtime

import (
	"fmt"
	"strings"

	mcptools "github.com/tingly-dev/tingly-box/internal/mcp/tools"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// CheckToolScope refuses a GitHub call that leaves the profile's repository
// scope. It runs on the call path, not only when listing tools, so guessed
// arguments cannot widen access. Other sources and unscoped profiles pass.
func (r *Runtime) CheckToolScope(profileID, normalizedName string, args map[string]any) error {
	if r == nil || profileID == "" {
		return nil
	}
	cfg := r.GetConfig()
	if cfg == nil {
		return nil
	}
	sourceID, tool, ok := ParseNormalizedToolName(normalizedName)
	if !ok {
		return nil
	}
	var source *typ.MCPSourceConfig
	for i := range cfg.Sources {
		if cfg.Sources[i].ID == sourceID {
			source = &cfg.Sources[i]
			break
		}
	}
	if source == nil || source.Preset != mcptools.GitHubPreset {
		return nil
	}
	profile, ok := profileByID(cfg, profileID)
	if !ok || len(profile.GitHubRepos) == 0 || !mcptools.GitHubToolNeedsRepo(tool) {
		return nil
	}
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	if owner == "" || repo == "" {
		return fmt.Errorf("%s needs owner and repo; this MCP profile is limited to %s", tool, strings.Join(profile.GitHubRepos, ", "))
	}
	if !mcptools.GitHubRepoAllowed(profile.GitHubRepos, owner, repo) {
		return fmt.Errorf("repository %s/%s is outside this MCP profile's scope (%s)", owner, repo, strings.Join(profile.GitHubRepos, ", "))
	}
	return nil
}
