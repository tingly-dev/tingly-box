package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	tomlpkg "github.com/pelletier/go-toml/v2"
	yamlpkg "gopkg.in/yaml.v3"
)

// Client config status: does a tool's config file on this machine match what
// its Auto Config would write now? Only the gateway-owned values are
// compared — whether the file routes through this gateway, and the model
// list derived from the rules. User preferences and the base URL's host
// (which depends on how the UI was reached) are not. See
// .design/agent-page-redesign.md §3.2.
const (
	ClientConfigNotApplied = "not_applied"
	ClientConfigApplied    = "applied"
	ClientConfigOutdated   = "outdated"
)

// ClientConfigDiff is one gateway-owned value that differs from what would be
// written now.
type ClientConfigDiff struct {
	Key      string
	Applied  string
	Expected string
}

func joinModels(models []string) string {
	sorted := slices.Clone(models)
	slices.Sort(sorted)
	return strings.Join(sorted, ", ")
}

func stateFor(diffs []ClientConfigDiff) string {
	if len(diffs) > 0 {
		return ClientConfigOutdated
	}
	return ClientConfigApplied
}

func routesTo(baseURL, endpoint string) bool {
	return strings.HasSuffix(strings.TrimRight(baseURL, "/"), endpoint)
}

// ---- Codex ----

// CodexAppliedState is what the status check needs from ~/.codex.
type CodexAppliedState struct {
	Managed           bool           // model_provider / provider stanza is tingly-box
	BaseURL           string         // [model_providers.tingly-box].base_url
	Model             string         // top-level model
	CatalogModels     []string       // slugs in the tingly model catalog, nil if none
	ContextWindows    map[string]int // catalog context_window per slug
	MaxContextWindows map[string]int // catalog max_context_window per slug, when the entry has one
}

// CompareCodexConfig compares the applied Codex state with the models, context
// windows (BuildContextWindowsFromRules) and ceilings
// (BuildMaxContextWindowsFromRules) the current rules would produce. A model
// missing from contextWindows is expected at the default window, and one
// missing from maxContextWindows at a ceiling equal to its window.
func CompareCodexConfig(applied CodexAppliedState, models []string, contextWindows, maxContextWindows map[string]int) (string, []ClientConfigDiff) {
	if !applied.Managed || !routesTo(applied.BaseURL, "/tingly/codex") {
		return ClientConfigNotApplied, nil
	}
	var diffs []ClientConfigDiff
	if len(models) > 0 && applied.Model != models[0] {
		diffs = append(diffs, ClientConfigDiff{Key: "model", Applied: applied.Model, Expected: models[0]})
	}
	if applied.CatalogModels != nil {
		if joinModels(applied.CatalogModels) != joinModels(models) {
			diffs = append(diffs, ClientConfigDiff{Key: "models", Applied: joinModels(applied.CatalogModels), Expected: joinModels(models)})
		}
		for _, m := range models {
			have, ok := applied.ContextWindows[m]
			if !ok {
				continue // already reported as a missing model
			}
			want := codexDefaultContextWindow
			if cw, ok := contextWindows[m]; ok {
				want = cw
			}
			if have != want {
				diffs = append(diffs, ClientConfigDiff{Key: "context_window[" + m + "]", Applied: fmt.Sprint(have), Expected: fmt.Sprint(want)})
			}
			// A ceiling that doesn't match lets Codex's model_context_window
			// raise the window past what the upstream accepts, or stops it short.
			wantMax := want
			if mw, ok := maxContextWindows[m]; ok && mw > want {
				wantMax = mw
			}
			if haveMax, ok := applied.MaxContextWindows[m]; ok && haveMax != wantMax {
				diffs = append(diffs, ClientConfigDiff{Key: "max_context_window[" + m + "]", Applied: fmt.Sprint(haveMax), Expected: fmt.Sprint(wantMax)})
			}
		}
	}
	return stateFor(diffs), diffs
}

// ReadCodexAppliedState reads ~/.codex/config.toml and, when it points at
// one, the tingly model catalog. A missing or unparseable file is simply
// "not managed".
func ReadCodexAppliedState() (CodexAppliedState, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return CodexAppliedState{}, fmt.Errorf("failed to get home directory: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(homeDir, ".codex", "config.toml"))
	if err != nil {
		return CodexAppliedState{}, nil
	}
	cfg := map[string]interface{}{}
	if err := tomlpkg.Unmarshal(data, &cfg); err != nil {
		return CodexAppliedState{}, nil
	}
	state := CodexAppliedState{Managed: isTinglyManagedCodexConfig(cfg)}
	state.Model, _ = cfg["model"].(string)
	if providers, ok := cfg["model_providers"].(map[string]interface{}); ok {
		if stanza, ok := providers[codexGatewayProviderName].(map[string]interface{}); ok {
			state.BaseURL, _ = stanza["base_url"].(string)
		}
	}
	if catalogPath, ok := cfg["model_catalog_json"].(string); ok && catalogPath != "" {
		if raw, err := os.ReadFile(catalogPath); err == nil {
			var catalog struct {
				Models []struct {
					Slug             string `json:"slug"`
					ContextWindow    int    `json:"context_window"`
					MaxContextWindow *int   `json:"max_context_window"`
				} `json:"models"`
			}
			if json.Unmarshal(raw, &catalog) == nil {
				state.CatalogModels = []string{}
				state.ContextWindows = map[string]int{}
				state.MaxContextWindows = map[string]int{}
				for _, m := range catalog.Models {
					state.CatalogModels = append(state.CatalogModels, m.Slug)
					state.ContextWindows[m.Slug] = m.ContextWindow
					if m.MaxContextWindow != nil {
						state.MaxContextWindows[m.Slug] = *m.MaxContextWindow
					}
				}
			}
		}
	}
	return state, nil
}

// ---- DeepSeek Harness (dsh) ----

// DshAppliedState is what the status check needs from $DSH_HOME/settings.yaml.
type DshAppliedState struct {
	Managed bool // has the llm-pi-ai.providers.tingly-box stanza
	BaseURL string
	Models  []string
}

// CompareDshConfig compares the applied dsh provider stanza with the models
// the current rules would produce.
func CompareDshConfig(applied DshAppliedState, models []string) (string, []ClientConfigDiff) {
	if !applied.Managed || !routesTo(applied.BaseURL, "/tingly/dsh") {
		return ClientConfigNotApplied, nil
	}
	var diffs []ClientConfigDiff
	if joinModels(applied.Models) != joinModels(models) {
		diffs = append(diffs, ClientConfigDiff{Key: "models", Applied: joinModels(applied.Models), Expected: joinModels(models)})
	}
	return stateFor(diffs), diffs
}

// ReadDshAppliedState reads the tingly-box provider stanza from
// $DSH_HOME/settings.yaml.
func ReadDshAppliedState() (DshAppliedState, error) {
	dshHome, err := dshHomeDir()
	if err != nil {
		return DshAppliedState{}, err
	}
	data, err := os.ReadFile(filepath.Join(dshHome, "settings.yaml"))
	if err != nil {
		return DshAppliedState{}, nil
	}
	cfg := map[string]interface{}{}
	if err := yamlpkg.Unmarshal(data, &cfg); err != nil {
		return DshAppliedState{}, nil
	}
	stanza, ok := dshProviderStanza(cfg)
	if !ok {
		return DshAppliedState{}, nil
	}
	state := DshAppliedState{Managed: true, Models: []string{}}
	state.BaseURL, _ = stanza["baseURL"].(string)
	if entries, ok := stanza["models"].([]interface{}); ok {
		for _, e := range entries {
			if m, ok := e.(map[string]interface{}); ok {
				if id, ok := m["id"].(string); ok {
					state.Models = append(state.Models, id)
				}
			}
		}
	}
	return state, nil
}
