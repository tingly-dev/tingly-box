package catalog

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// API maxima must not overwrite the smaller default Codex context windows.
func TestOpenAICodexCatalogLimits(t *testing.T) {
	data, err := os.ReadFile("providers.json")
	require.NoError(t, err)
	var doc struct {
		Providers map[string]struct {
			Models []ModelInfo `json:"models"`
		} `json:"providers"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	find := func(provider, id string) ModelInfo {
		for _, m := range doc.Providers[provider].Models {
			if m.ID == id {
				return m
			}
		}
		t.Fatalf("missing %s/%s", provider, id)
		return ModelInfo{}
	}
	for _, id := range []string{"gpt-6.2-astra", "gpt-6.2-sol"} {
		m := find("openai-com", id)
		require.Equal(t, 1050000, m.Context)
		require.Equal(t, 128000, m.MaxOutput)
		require.Equal(t, []string{"responses"}, m.OpenAIEndpoints)
	}
	for id, limit := range map[string]int{"gpt-6.2-astra": 272000, "gpt-6.2-sol": 500000, "gpt-6.1-sol": 500000} {
		for _, suffix := range []string{"", ":fast"} {
			m := find("codex", id+suffix)
			require.Equal(t, limit, m.Context)
			require.Equal(t, []string{"responses"}, m.OpenAIEndpoints)
		}
	}
	// Codex ChatGPT-login retirements must not remove API-key models.
	require.Equal(t, "gpt-5.5", find("openai-com", "gpt-5.5").ID)
}
