// Package catalog is the per-vendor model capability catalog: what each model
// can do, declared once per model family, independent of which provider serves
// it. It complements internal/data/providers.json, which is the offering
// registry (who serves which model, at which endpoint, with what limits) —
// capability facts live here, deployment facts live there.
//
// Layout: one <vendor>.models.json data file plus one <vendor>.go loader per
// vendor (claude.models.json + claude.go today; openai/gemini can follow the
// same pattern). Each file only carries the fields its loader actually
// consumes — deliberately not a mirror of the vendor's full /v1/models
// response, whose unused fields (display names, dates, unrelated capability
// flags) are dead weight.
//
// The `reasoning` block's shape and field names (supported_efforts, ...)
// follow OpenRouter's model-list schema rather than Anthropic's own nested
// `capabilities.effort.<level>.supported` tree. One field has no OpenRouter
// equivalent: `dialects`. OpenRouter's clients never see wire-protocol
// differences between backends — OpenRouter's own proxy absorbs that. This
// package IS that proxy layer for Anthropic, so it has to know which raw
// request shape a model accepts: "budget" (thinking.type=enabled +
// budget_tokens) and/or "adaptive" (thinking.type=adaptive). Models with no
// `reasoning` block at all do not support extended thinking.
//
// Update the JSON when new models land instead of hardcoding model names in
// code; the completeness test in this package fails when providers.json
// offers a Claude model this catalog does not describe.
//
// claude.models.snapshot.json is a separate, unmodified mirror of Anthropic's
// actual /v1/models response — the ground truth to cross-check
// claude.models.json against when adding or revising an entry. It is not
// embedded and no code reads it; it goes stale as new models ship (it
// currently covers 10 of the 19 models in claude.models.json — models added
// afterward had their reasoning capabilities inferred from the newest
// snapshot entry rather than sourced, see .design/model-data.md) and should
// be refreshed by hand from the live API when convenient, not on every edit.
package catalog

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"sync"
)

//go:embed claude.models.json
var claudeModelsJSON []byte

// ClaudeThinkingCaps describes which thinking dialects a cataloged Claude
// model accepts.
type ClaudeThinkingCaps struct {
	// ThinkingEnabled: accepts thinking.type=enabled with budget_tokens.
	ThinkingEnabled bool
	// ThinkingAdaptive: accepts thinking.type=adaptive.
	ThinkingAdaptive bool
	// EffortLevels: supported output_config.effort values (e.g. "low", "max").
	// Empty means the model has no effort support.
	EffortLevels map[string]bool
}

// SupportsEffort reports whether the model accepts output_config.effort at all.
func (c ClaudeThinkingCaps) SupportsEffort() bool { return len(c.EffortLevels) > 0 }

type catalogModel struct {
	ID        string `json:"id"`
	Reasoning *struct {
		Dialects         []string `json:"dialects"`
		SupportedEfforts []string `json:"supported_efforts"`
	} `json:"reasoning"`
}

type claudeCapsEntry struct {
	key  string
	caps ClaudeThinkingCaps
}

var claudeCapsIndex = sync.OnceValue(buildClaudeCapsIndex)

var claudeDateSuffixRE = regexp.MustCompile(`-\d{8}$`)

// buildClaudeCapsIndex flattens the catalog into substring match keys: each
// model is indexed under its full id and its date-stripped family name, so
// bare names ("claude-opus-4-5"), dated ids, and cloud-provider decorations
// ("us.anthropic.claude-sonnet-4-5-20250929-v1:0") all resolve. Keys are
// sorted longest-first so the most specific entry wins (e.g.
// "claude-sonnet-4-6" before "claude-sonnet-4").
func buildClaudeCapsIndex() []claudeCapsEntry {
	var models []catalogModel
	if err := json.Unmarshal(claudeModelsJSON, &models); err != nil {
		return nil
	}

	var entries []claudeCapsEntry
	seen := map[string]bool{}
	add := func(key string, caps ClaudeThinkingCaps) {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		entries = append(entries, claudeCapsEntry{key: key, caps: caps})
	}

	for _, m := range models {
		var caps ClaudeThinkingCaps
		if m.Reasoning != nil {
			for _, d := range m.Reasoning.Dialects {
				switch d {
				case "budget":
					caps.ThinkingEnabled = true
				case "adaptive":
					caps.ThinkingAdaptive = true
				}
			}
			if len(m.Reasoning.SupportedEfforts) > 0 {
				caps.EffortLevels = make(map[string]bool, len(m.Reasoning.SupportedEfforts))
				for _, lvl := range m.Reasoning.SupportedEfforts {
					caps.EffortLevels[lvl] = true
				}
			}
		}
		add(m.ID, caps)
		add(claudeDateSuffixRE.ReplaceAllString(m.ID, ""), caps)
	}

	sort.SliceStable(entries, func(i, j int) bool { return len(entries[i].key) > len(entries[j].key) })
	return entries
}

// LookupClaudeThinkingCaps resolves a model name to the catalog's thinking
// capabilities. The longest catalog key contained in the (lowercased) name
// wins; ok=false when the model is not in the catalog.
func LookupClaudeThinkingCaps(model string) (ClaudeThinkingCaps, bool) {
	m := strings.ToLower(model)
	if m == "" {
		return ClaudeThinkingCaps{}, false
	}
	for _, e := range claudeCapsIndex() {
		if strings.Contains(m, e.key) {
			return e.caps, true
		}
	}
	return ClaudeThinkingCaps{}, false
}

// hasClaudeCatalogEntry is the strict identity check used by completeness
// tests. Provider decorations are normalized, but the resulting model id must
// equal a full or date-stripped catalog key; a future family must not inherit
// an older entry merely because runtime lookup allows decorated substrings.
func hasClaudeCatalogEntry(model string) bool {
	m, ok := normalizeClaudeCatalogID(model)
	if !ok {
		return false
	}
	for _, e := range claudeCapsIndex() {
		if m == e.key {
			return true
		}
	}
	return false
}

func normalizeClaudeCatalogID(model string) (string, bool) {
	m := strings.ToLower(strings.TrimSpace(model))
	start := strings.Index(m, "claude-")
	if start < 0 {
		return "", false
	}
	m = m[start:]
	m = strings.TrimSuffix(m, "-v1:0")
	m = strings.TrimSuffix(m, "-thinking")
	if at := strings.IndexByte(m, '@'); at >= 0 {
		m = m[:at]
	}
	return m, m != ""
}
