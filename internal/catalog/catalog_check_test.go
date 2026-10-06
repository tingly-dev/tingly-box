package catalog

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmbeddedCatalogsPassCheck(t *testing.T) {
	docs := EmbeddedCatalogJSON()
	require.Empty(t, CheckProviderCatalogJSON(docs[KindProviders]))
	require.Empty(t, CheckClaudeCatalogJSON(docs[KindClaudeModels]))
	require.Empty(t, CheckCrossCatalog(docs[KindProviders], docs[KindClaudeModels]))
}

func TestDetectCatalogKind(t *testing.T) {
	docs := EmbeddedCatalogJSON()
	for kind, data := range docs {
		got, err := DetectCatalogKind(data)
		require.NoError(t, err)
		require.Equal(t, kind, got)
	}
	_, err := DetectCatalogKind([]byte(`{"foo":1}`))
	require.Error(t, err)
	_, err = DetectCatalogKind([]byte(``))
	require.Error(t, err)
}

func TestCheckProviderCatalogJSONFindsProblems(t *testing.T) {
	issues := CheckProviderCatalogJSON([]byte(`{"providers":{
		"a":{"id":"b","name":"A","base_url_openai":"https://x","models":[{"id":"m"},{"id":"m"},{"id":""}]},
		"c":{"id":"c","name":"C"}
	}}`))
	var msgs []string
	for _, i := range issues {
		msgs = append(msgs, i.String())
	}
	all := strings.Join(msgs, "\n")
	require.Contains(t, all, `a: map key does not match id "b"`)
	require.Contains(t, all, `a: duplicate model id "m"`)
	require.Contains(t, all, `a: model with empty id`)
	require.Contains(t, all, `c: at least one base URL is required`)

	// Reference-only metadata is tolerated; type errors are not.
	require.Empty(t, CheckProviderCatalogJSON([]byte(`{"providers":{"a":{"id":"a","name":"A","base_url_openai":"x","sources":["s"]}}}`)))
	issues = CheckProviderCatalogJSON([]byte(`{"providers":{"a":{"id":"a","name":["A"]}}}`))
	require.Len(t, issues, 1)
	require.Contains(t, issues[0].Message, "parse")
}

func TestCheckClaudeCatalogJSONFindsProblems(t *testing.T) {
	issues := CheckClaudeCatalogJSON([]byte(`[
		{"id":"claude-a","reasoning":{"dialects":["weird"],"supported_efforts":["huge"]}},
		{"id":"claude-a"},
		{"id":"claude-b","reasoning":{}}
	]`))
	var all []string
	for _, i := range issues {
		all = append(all, i.String())
	}
	joined := strings.Join(all, "\n")
	require.Contains(t, joined, `unknown dialect "weird"`)
	require.Contains(t, joined, `unknown effort level "huge"`)
	require.Contains(t, joined, `claude-a: duplicate model id`)
	require.Contains(t, joined, `claude-b: reasoning block has no dialects`)
}

func TestCheckCrossCatalog(t *testing.T) {
	providers := []byte(`{"providers":{"p":{"id":"p","name":"P","base_url_openai":"x","models":[
		{"id":"claude-known-20250101"},{"id":"us.anthropic.claude-known-20250101-v1:0"},{"id":"claude-missing"},{"id":"gpt-4"}]}}}`)
	claude := []byte(`[{"id":"claude-known-20250101"}]`)
	issues := CheckCrossCatalog(providers, claude)
	require.Len(t, issues, 1)
	require.Equal(t, "p", issues[0].Subject)
	require.Contains(t, issues[0].Message, "claude-missing")
}
