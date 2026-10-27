package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The ai/agent copy of the catalog renderer must honor the same headroom
// prefs as internal/config (the two copies are intentionally duplicated).
func TestRenderCodexModelCatalogWithPrefs_Headroom(t *testing.T) {
	render := func(p *CodexPrefs) (effective, compact, window int) {
		raw, err := RenderCodexModelCatalogWithPrefs([]string{"m"}, nil, p)
		if err != nil {
			t.Fatal(err)
		}
		var c struct {
			Models []struct {
				ContextWindow int `json:"context_window"`
				Compact       int `json:"auto_compact_token_limit"`
				Effective     int `json:"effective_context_window_percent"`
			} `json:"models"`
		}
		if err := json.Unmarshal(raw, &c); err != nil {
			t.Fatal(err)
		}
		m := c.Models[0]
		return m.Effective, m.Compact, m.ContextWindow
	}

	if e, c, w := render(nil); e != 82 || c != w*75/100 {
		t.Errorf("defaults: effective=%d compact=%d window=%d", e, c, w)
	}
	if e, c, w := render(&CodexPrefs{EffectiveContextWindowPercent: "90", AutoCompactPercent: "70"}); e != 90 || c != w*70/100 {
		t.Errorf("custom: effective=%d compact=%d window=%d", e, c, w)
	}
	if e, c, w := render(&CodexPrefs{EffectiveContextWindowPercent: "60", AutoCompactPercent: "95"}); e != 60 || c != w*60/100 {
		t.Errorf("clamp: effective=%d compact=%d window=%d", e, c, w)
	}
}

// Per-model windows, ceilings and compact overrides reach the written
// catalog through CodexParams (the tingly-box CLI bridge's path), like the
// internal/config renderer.
func TestCodexConfig_Apply_PerModelWindows(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	prefs := DefaultCodexPrefs()
	prefs.ModelAutoCompactPercent = map[string]int{"ds": 60, "sol": 95, "plain": 10}
	prefs.ModelMaxContextWindow = map[string]int{"sol": 872000, "ds": 64000}
	_, err := (&CodexConfig{}).Apply(&CodexParams{
		CodexBaseURL:   "https://tingly.local/tingly/codex",
		APIKey:         "sk-test",
		Models:         []string{"ds", "sol", "plain"},
		Prefs:          prefs,
		ContextWindows: map[string]int{"ds": 128000, "sol": 272000},
		WriteCatalog:   true,
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(home, ".codex", "tingly-model-catalog.json"))
	if err != nil {
		t.Fatalf("catalog not written: %v", err)
	}
	var catalog struct {
		Models []struct {
			Slug    string `json:"slug"`
			Window  int    `json:"context_window"`
			Max     int    `json:"max_context_window"`
			Compact int    `json:"auto_compact_token_limit"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	want := map[string][3]int{
		"ds":    {128000, 128000, 76800},  // own 60%; a ceiling below the window is ignored
		"sol":   {272000, 872000, 223040}, // 95% capped to the usable 82%
		"plain": {200000, 200000, 150000}, // out-of-range override → shared 75%
	}
	for _, m := range catalog.Models {
		if got := [3]int{m.Window, m.Max, m.Compact}; got != want[m.Slug] {
			t.Errorf("%s: window/max/compact = %v, want %v", m.Slug, got, want[m.Slug])
		}
	}
	if len(catalog.Models) != len(want) {
		t.Errorf("catalog has %d models, want %d", len(catalog.Models), len(want))
	}
}
