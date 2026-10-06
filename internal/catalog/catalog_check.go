package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// CatalogKind identifies which catalog a JSON document is.
type CatalogKind string

const (
	// KindProviders is providers.json (the offering registry).
	KindProviders CatalogKind = "providers"
	// KindClaudeModels is claude.models.json (the Claude capability catalog).
	KindClaudeModels CatalogKind = "claude-models"
)

// CheckIssue is one validity problem found in a catalog.
type CheckIssue struct {
	Catalog CatalogKind `json:"catalog"`
	// Subject is the provider or model the problem is about; empty for
	// document-level problems (e.g. unparseable JSON).
	Subject string `json:"subject,omitempty"`
	Message string `json:"message"`
}

func (i CheckIssue) String() string {
	if i.Subject == "" {
		return fmt.Sprintf("[%s] %s", i.Catalog, i.Message)
	}
	return fmt.Sprintf("[%s] %s: %s", i.Catalog, i.Subject, i.Message)
}

// EmbeddedCatalogJSON returns the catalog documents compiled into the binary.
func EmbeddedCatalogJSON() map[CatalogKind][]byte {
	return map[CatalogKind][]byte{
		KindProviders:    embeddedTemplatesJSON,
		KindClaudeModels: claudeModelsJSON,
	}
}

// DetectCatalogKind tells the catalogs apart by shape: providers.json is an
// object with a "providers" key, claude.models.json is a top-level array.
func DetectCatalogKind(data []byte) (CatalogKind, error) {
	trimmed := bytes.TrimSpace(data)
	switch {
	case len(trimmed) == 0:
		return "", fmt.Errorf("empty document")
	case trimmed[0] == '[':
		return KindClaudeModels, nil
	case trimmed[0] == '{':
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &probe); err != nil {
			return "", fmt.Errorf("invalid JSON: %w", err)
		}
		if _, ok := probe["providers"]; ok {
			return KindProviders, nil
		}
	}
	return "", fmt.Errorf("not a recognized catalog (expected providers.json object with \"providers\", or claude.models.json array)")
}

func strictDecode(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("unexpected data after top-level value")
	}
	return nil
}

// CheckProviderCatalogJSON validates a providers.json document: it must decode
// (type errors count), map key == id, ValidateProviderCatalog per provider, and
// model ids are unique and non-empty within each provider. Unknown fields are
// deliberately tolerated: providers.json carries reference-only metadata
// (sources, note, last_updated, ...) that Go drops by design.
func CheckProviderCatalogJSON(data []byte) []CheckIssue {
	var reg ProviderCatalogRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		return []CheckIssue{{Catalog: KindProviders, Message: fmt.Sprintf("parse: %v", err)}}
	}
	var issues []CheckIssue
	add := func(subject, format string, args ...any) {
		issues = append(issues, CheckIssue{Catalog: KindProviders, Subject: subject, Message: fmt.Sprintf(format, args...)})
	}
	if len(reg.Providers) == 0 {
		add("", "no providers defined")
	}
	for _, key := range sortedKeys(reg.Providers) {
		p := reg.Providers[key]
		if p == nil {
			add(key, "provider entry is null")
			continue
		}
		if p.ID != key {
			add(key, "map key does not match id %q", p.ID)
		}
		if err := ValidateProviderCatalog(p); err != nil {
			add(key, "%v", err)
		}
		seen := map[string]bool{}
		for _, m := range p.Models {
			switch {
			case m.ID == "":
				add(key, "model with empty id")
			case seen[m.ID]:
				add(key, "duplicate model id %q", m.ID)
			}
			seen[m.ID] = true
		}
	}
	return issues
}

var (
	validClaudeDialects = map[string]bool{"budget": true, "adaptive": true}
	validClaudeEfforts  = map[string]bool{"low": true, "medium": true, "high": true, "xhigh": true, "max": true}
)

// CheckClaudeCatalogJSON validates a claude.models.json document: strict
// decoding, unique non-empty ids, and known dialect / effort values.
func CheckClaudeCatalogJSON(data []byte) []CheckIssue {
	var models []catalogModel
	if err := strictDecode(data, &models); err != nil {
		return []CheckIssue{{Catalog: KindClaudeModels, Message: fmt.Sprintf("parse: %v", err)}}
	}
	var issues []CheckIssue
	add := func(subject, format string, args ...any) {
		issues = append(issues, CheckIssue{Catalog: KindClaudeModels, Subject: subject, Message: fmt.Sprintf(format, args...)})
	}
	if len(models) == 0 {
		add("", "no models defined")
	}
	seen := map[string]bool{}
	for _, m := range models {
		if m.ID == "" {
			add("", "model with empty id")
			continue
		}
		if seen[m.ID] {
			add(m.ID, "duplicate model id")
		}
		seen[m.ID] = true
		r := m.Reasoning
		if r == nil {
			continue
		}
		if len(r.Dialects) == 0 {
			add(m.ID, "reasoning block has no dialects")
		}
		for _, d := range r.Dialects {
			if !validClaudeDialects[d] {
				add(m.ID, "unknown dialect %q (want budget|adaptive)", d)
			}
		}
		for _, e := range r.SupportedEfforts {
			if !validClaudeEfforts[e] {
				add(m.ID, "unknown effort level %q (want low|medium|high|xhigh|max)", e)
			}
		}
	}
	return issues
}

// CheckCrossCatalog verifies that every Claude model offered by a provider in
// providers.json has an entry in the Claude capability catalog. Documents that
// failed to parse yield no cross-catalog issues (their own check reports why).
func CheckCrossCatalog(providersJSON, claudeJSON []byte) []CheckIssue {
	var reg ProviderCatalogRegistry
	if err := json.Unmarshal(providersJSON, &reg); err != nil {
		return nil
	}
	var models []catalogModel
	if err := json.Unmarshal(claudeJSON, &models); err != nil {
		return nil
	}
	known := map[string]bool{}
	for _, m := range models {
		id := strings.ToLower(m.ID)
		known[id] = true
		known[claudeDateSuffixRE.ReplaceAllString(id, "")] = true
	}
	var issues []CheckIssue
	for _, key := range sortedKeys(reg.Providers) {
		p := reg.Providers[key]
		if p == nil {
			continue
		}
		for _, m := range p.Models {
			if !strings.Contains(strings.ToLower(m.ID), "claude") {
				continue
			}
			if id, ok := normalizeClaudeCatalogID(m.ID); !ok || !known[id] {
				issues = append(issues, CheckIssue{
					Catalog: KindProviders,
					Subject: key,
					Message: fmt.Sprintf("offers Claude model %q with no entry in claude.models.json", m.ID),
				})
			}
		}
	}
	return issues
}

// CheckCatalogs runs every check over the given documents (keyed by kind):
// each catalog on its own, then consistency across them.
func CheckCatalogs(docs map[CatalogKind][]byte) []CheckIssue {
	providers, claude := docs[KindProviders], docs[KindClaudeModels]
	issues := CheckProviderCatalogJSON(providers)
	issues = append(issues, CheckClaudeCatalogJSON(claude)...)
	return append(issues, CheckCrossCatalog(providers, claude)...)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
