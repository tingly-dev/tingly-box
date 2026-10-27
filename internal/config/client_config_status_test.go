package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompareCodexConfig(t *testing.T) {
	applied := CodexAppliedState{
		Managed:        true,
		BaseURL:        "http://localhost:12580/tingly/codex",
		Model:          "gpt-5.6-luna",
		CatalogModels:  []string{"gpt-5.6-luna", "gpt-5.6-terra"},
		ContextWindows: map[string]int{"gpt-5.6-luna": codexDefaultContextWindow, "gpt-5.6-terra": codexDefaultContextWindow},
	}

	state, diffs := CompareCodexConfig(applied, []string{"gpt-5.6-luna", "gpt-5.6-terra"}, nil, nil)
	assert.Equal(t, ClientConfigApplied, state)
	assert.Empty(t, diffs)

	// A rule was added and luna switched to 1M after the last apply.
	state, diffs = CompareCodexConfig(applied, []string{"gpt-5.6-luna", "gpt-5.6-terra", "gpt-5.6-sol"}, map[string]int{"gpt-5.6-luna": 1000000}, nil)
	assert.Equal(t, ClientConfigOutdated, state)
	assert.Equal(t, []ClientConfigDiff{
		{Key: "models", Applied: "gpt-5.6-luna, gpt-5.6-terra", Expected: "gpt-5.6-luna, gpt-5.6-sol, gpt-5.6-terra"},
		{Key: "context_window[gpt-5.6-luna]", Applied: "200000", Expected: "1000000"},
	}, diffs)

	state, _ = CompareCodexConfig(CodexAppliedState{}, []string{"x"}, nil, nil)
	assert.Equal(t, ClientConfigNotApplied, state)
	state, _ = CompareCodexConfig(CodexAppliedState{Managed: true, BaseURL: "https://api.openai.com/v1"}, []string{"x"}, nil, nil)
	assert.Equal(t, ClientConfigNotApplied, state)
}

// max_context_window is compared too: its expected value is the ceiling the
// rules give (272K window / 872K ceiling on a ChatGPT-backed model), else the
// window. An applied catalog without the key (hand-edited) isn't judged on it.
func TestCompareCodexConfig_MaxContextWindow(t *testing.T) {
	applied := CodexAppliedState{
		Managed:           true,
		BaseURL:           "http://localhost:12580/tingly/codex",
		Model:             "sol",
		CatalogModels:     []string{"sol", "ds", "bare"},
		ContextWindows:    map[string]int{"sol": 272000, "ds": 128000, "bare": 200000},
		MaxContextWindows: map[string]int{"sol": 272000, "ds": 1000000},
	}
	windows := map[string]int{"sol": 272000, "ds": 128000}

	state, diffs := CompareCodexConfig(applied, []string{"sol", "ds", "bare"}, windows, map[string]int{"sol": 872000})
	assert.Equal(t, ClientConfigOutdated, state)
	assert.Equal(t, []ClientConfigDiff{
		{Key: "max_context_window[sol]", Applied: "272000", Expected: "872000"},
		{Key: "max_context_window[ds]", Applied: "1000000", Expected: "128000"},
	}, diffs)

	applied.MaxContextWindows = map[string]int{"sol": 872000, "ds": 128000}
	state, diffs = CompareCodexConfig(applied, []string{"sol", "ds", "bare"}, windows, map[string]int{"sol": 872000})
	assert.Equal(t, ClientConfigApplied, state, "%v", diffs)

	// A ceiling below the window can't be written, so it isn't expected.
	state, diffs = CompareCodexConfig(applied, []string{"sol", "ds", "bare"}, windows, map[string]int{"sol": 872000, "ds": 64000})
	assert.Equal(t, ClientConfigApplied, state, "%v", diffs)
}

func TestCompareDshConfig(t *testing.T) {
	applied := DshAppliedState{Managed: true, BaseURL: "http://127.0.0.1:12580/tingly/dsh", Models: []string{"deepseek-v4-pro"}}

	state, diffs := CompareDshConfig(applied, []string{"deepseek-v4-pro"})
	assert.Equal(t, ClientConfigApplied, state)
	assert.Empty(t, diffs)

	state, diffs = CompareDshConfig(applied, []string{"deepseek-v4-pro", "deepseek-v4-flash"})
	assert.Equal(t, ClientConfigOutdated, state)
	assert.Equal(t, "deepseek-v4-flash, deepseek-v4-pro", diffs[0].Expected)

	state, _ = CompareDshConfig(DshAppliedState{}, []string{"x"})
	assert.Equal(t, ClientConfigNotApplied, state)
}

// The reader must agree with the writer: applying and reading back is "applied".
func TestCodexStatusRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	models := []string{"gpt-5.6-luna", "gpt-5.6-terra"}
	windows := map[string]int{"gpt-5.6-terra": 1000000}

	_, err := ApplyCodexConfigWithContextWindows("http://localhost:12580/tingly/codex", models, DefaultCodexPrefs(), true, windows, "")
	assert.NoError(t, err)
	applied, err := ReadCodexAppliedState()
	assert.NoError(t, err)

	state, diffs := CompareCodexConfig(applied, models, windows, nil)
	assert.Equal(t, ClientConfigApplied, state, "%v", diffs)
	state, _ = CompareCodexConfig(applied, models, nil, nil)
	assert.Equal(t, ClientConfigOutdated, state, "dropping 1M on terra must show up")
}

// Ceilings round-trip: what the renderer writes is what the status check
// expects, and a changed ceiling shows up.
func TestCodexStatusRoundTrip_MaxContextWindow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	models := []string{"sol", "ds"}
	windows := map[string]int{"sol": 272000, "ds": 128000}
	ceilings := map[string]int{"sol": 872000}
	prefs := DefaultCodexPrefs()
	prefs.ModelMaxContextWindow = ceilings

	_, err := ApplyCodexConfigWithContextWindows("http://localhost:12580/tingly/codex", models, prefs, true, windows, "")
	assert.NoError(t, err)
	applied, err := ReadCodexAppliedState()
	assert.NoError(t, err)
	assert.Equal(t, map[string]int{"sol": 872000, "ds": 128000}, applied.MaxContextWindows)

	state, diffs := CompareCodexConfig(applied, models, windows, ceilings)
	assert.Equal(t, ClientConfigApplied, state, "%v", diffs)
	state, diffs = CompareCodexConfig(applied, models, windows, nil)
	assert.Equal(t, ClientConfigOutdated, state)
	assert.Equal(t, []ClientConfigDiff{{Key: "max_context_window[sol]", Applied: "872000", Expected: "272000"}}, diffs)
}

func TestDshStatusRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("DSH_HOME", home)
	models := []string{"deepseek-v4-pro"}

	_, err := ApplyDshSettings("http://localhost:12580/tingly/dsh", models, DefaultDshPrefs())
	assert.NoError(t, err)
	applied, err := ReadDshAppliedState()
	assert.NoError(t, err)

	state, diffs := CompareDshConfig(applied, models)
	assert.Equal(t, ClientConfigApplied, state, "%v", diffs)
}
