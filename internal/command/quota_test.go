package command

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestFilterQuotaRelevant_ExcludesVirtualModels guards the fix: builtin
// vmodel providers (auth type vmodel) never make an outbound call, so they
// can never report quota — they used to show up in `quota` / `quota --all`
// as a permanent "Error: no data" row regardless of provider category.
func TestFilterQuotaRelevant_ExcludesVirtualModels(t *testing.T) {
	providers := []*typ.Provider{
		{Name: "vmodel-anthropic", AuthType: typ.AuthTypeVirtual},
		{Name: "vmodel-openai", AuthType: typ.AuthTypeVirtual},
		{Name: "real-api-key", AuthType: typ.AuthTypeAPIKey},
		{Name: "real-oauth", AuthType: typ.AuthTypeOAuth},
		{Name: "legacy-empty-authtype"}, // AuthType == "" defaults to api_key
	}

	got := filterQuotaRelevant(providers)

	names := make(map[string]bool, len(got))
	for _, p := range got {
		if p.IsVirtual() {
			t.Errorf("filterQuotaRelevant() kept virtual-model provider %q", p.Name)
		}
		names[p.Name] = true
	}
	for _, want := range []string{"real-api-key", "real-oauth", "legacy-empty-authtype"} {
		if !names[want] {
			t.Errorf("filterQuotaRelevant() dropped non-virtual provider %q", want)
		}
	}
	if len(got) != 3 {
		t.Errorf("filterQuotaRelevant() returned %d providers, want 3", len(got))
	}
}
