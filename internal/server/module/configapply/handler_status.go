package configapply

import (
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
)

// Client config status: is the tool's config on this machine what the
// gateway would write now? Mode changes, 1M toggles and request-model
// renames all change the expected values without touching the file, so the
// Agent page shows this instead of a one-off "please reapply" toast
// (.design/agent-page-redesign.md §3.2).
const (
	ClientConfigNotApplied = "not_applied" // no file, or it doesn't route through this gateway
	ClientConfigApplied    = "applied"     // routes through this gateway with the expected values
	ClientConfigOutdated   = "outdated"    // routes through this gateway, but some values differ
)

// claudeRoutedKeys are the settings.json env keys the gateway owns: the
// token and the six model slots. ANTHROPIC_BASE_URL is only checked for
// pointing at this gateway's Claude Code endpoint — its host legitimately
// differs by how the UI was reached (localhost vs 127.0.0.1 vs a proxy).
var claudeRoutedKeys = []string{
	"ANTHROPIC_AUTH_TOKEN",
	"ANTHROPIC_MODEL",
	"ANTHROPIC_DEFAULT_HAIKU_MODEL",
	"ANTHROPIC_DEFAULT_SONNET_MODEL",
	"ANTHROPIC_DEFAULT_OPUS_MODEL",
	"ANTHROPIC_DEFAULT_FABLE_MODEL",
	"CLAUDE_CODE_SUBAGENT_MODEL",
}

const claudeCodeEndpointSuffix = "/tingly/claude_code"

// ClientConfigDifference is one value that differs from what would be applied now.
type ClientConfigDifference struct {
	Key      string `json:"key"`
	Applied  string `json:"applied"`
	Expected string `json:"expected"`
}

// ClientConfigStatusResponse reports how a tool's on-disk config relates to
// the gateway's current state.
type ClientConfigStatusResponse struct {
	Success     bool                     `json:"success"`
	State       string                   `json:"state" example:"outdated"`
	Path        string                   `json:"path,omitempty" example:"~/.claude/settings.json"`
	Differences []ClientConfigDifference `json:"differences"`
	Error       string                   `json:"error,omitempty"`
}

// maskSecret keeps a token recognisable without echoing it back.
func maskSecret(s string) string {
	if len(s) <= 12 {
		if s == "" {
			return ""
		}
		return "••••"
	}
	return s[:8] + "…" + s[len(s)-4:]
}

// compareClaudeCodeEnv decides the status from the env found in
// settings.json (exists=false when there is no file) and the env the
// gateway would write now.
func compareClaudeCodeEnv(exists bool, applied, expected map[string]string) (string, []ClientConfigDifference) {
	diffs := []ClientConfigDifference{}
	if !exists || !strings.HasSuffix(strings.TrimRight(applied["ANTHROPIC_BASE_URL"], "/"), claudeCodeEndpointSuffix) {
		return ClientConfigNotApplied, diffs
	}
	for _, key := range claudeRoutedKeys {
		a, e := applied[key], expected[key]
		if a == e {
			continue
		}
		if key == "ANTHROPIC_AUTH_TOKEN" {
			a, e = maskSecret(a), maskSecret(e)
		}
		diffs = append(diffs, ClientConfigDifference{Key: key, Applied: a, Expected: e})
	}
	sort.Slice(diffs, func(i, j int) bool { return diffs[i].Key < diffs[j].Key })
	if len(diffs) > 0 {
		return ClientConfigOutdated, diffs
	}
	return ClientConfigApplied, diffs
}

// GetClaudeConfigStatus compares ~/.claude/settings.json with what Auto
// Config would write for the current mode and rules.
func (h *Handler) GetClaudeConfigStatus(c *gin.Context) {
	snapshot, err := agent.ReadMainClaudeCodeSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ClientConfigStatusResponse{Error: err.Error()})
		return
	}
	lines, err := tbclient.NewTBClient(h.config).GetClaudeCodeEnv(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, ClientConfigStatusResponse{Error: err.Error()})
		return
	}
	expected := make(map[string]string, len(lines))
	for _, line := range lines {
		if k, v, ok := strings.Cut(line, "="); ok {
			expected[k] = v
		}
	}
	state, diffs := compareClaudeCodeEnv(snapshot.Exists, snapshot.Env, expected)
	c.JSON(http.StatusOK, ClientConfigStatusResponse{
		Success:     true,
		State:       state,
		Path:        "~/.claude/settings.json",
		Differences: diffs,
	})
}

func toDifferences(diffs []config.ClientConfigDiff) []ClientConfigDifference {
	out := make([]ClientConfigDifference, 0, len(diffs))
	for _, d := range diffs {
		out = append(out, ClientConfigDifference{Key: d.Key, Applied: d.Applied, Expected: d.Expected})
	}
	return out
}

// GetCodexConfigStatus compares ~/.codex (config.toml + the tingly model
// catalog) with the models and context windows the Codex rules produce now.
func (h *Handler) GetCodexConfigStatus(c *gin.Context) {
	applied, err := config.ReadCodexAppliedState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ClientConfigStatusResponse{Error: err.Error()})
		return
	}
	state, diffs := config.CompareCodexConfig(applied, collectCodexRuleModels(h.config), config.BuildContextWindowsFromRules(h.config))
	c.JSON(http.StatusOK, ClientConfigStatusResponse{
		Success:     true,
		State:       state,
		Path:        "~/.codex/config.toml",
		Differences: toDifferences(diffs),
	})
}

// GetDshConfigStatus compares $DSH_HOME/settings.yaml with the models the dsh
// rules produce now.
func (h *Handler) GetDshConfigStatus(c *gin.Context) {
	applied, err := config.ReadDshAppliedState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ClientConfigStatusResponse{Error: err.Error()})
		return
	}
	state, diffs := config.CompareDshConfig(applied, collectDshRuleModels(h.config))
	c.JSON(http.StatusOK, ClientConfigStatusResponse{
		Success:     true,
		State:       state,
		Path:        "~/.dsh/settings.yaml",
		Differences: toDifferences(diffs),
	})
}
