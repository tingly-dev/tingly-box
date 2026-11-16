package agent

import (
	"encoding/json"
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
