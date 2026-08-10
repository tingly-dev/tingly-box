package catalog

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClaudeCatalogCoversProvidersJSON is the completeness invariant between
// the offering registry and this capability catalog: every Claude model any
// provider offers must match a catalog entry after known provider decorations
// are removed. When this fails, add the new model to claude.models.json — do
// not let a future family pass through runtime substring matching.
func TestClaudeCatalogCoversProvidersJSON(t *testing.T) {
	raw, err := os.ReadFile("../../data/providers.json")
	require.NoError(t, err)

	var doc struct {
		Providers map[string]struct {
			VendorFamily string `json:"vendor_family"`
			Models       []struct {
				ID string `json:"id"`
			} `json:"models"`
		} `json:"providers"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))

	checked := 0
	for providerID, p := range doc.Providers {
		for _, m := range p.Models {
			if !strings.Contains(strings.ToLower(m.ID), "claude") {
				continue
			}
			checked++
			require.True(t, hasClaudeCatalogEntry(m.ID),
				"provider %q offers %q but the Claude catalog has no entry for it — add it to claude.models.json",
				providerID, m.ID)
		}
	}
	require.Greater(t, checked, 0, "expected providers.json to offer Claude models")
}
