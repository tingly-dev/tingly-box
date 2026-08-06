package protocoltest

import (
	"os"
	"path/filepath"
	"testing"
)

func writeProvidersYAML(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "providers.yaml")
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	return p
}

func TestLoadProvidersConfigExpandsEnv(t *testing.T) {
	t.Setenv("TB_SHARED_KEY", "sk-shared-xyz")
	t.Setenv("TB_OPENAI_BASE", "https://api.openai.com")

	p := writeProvidersYAML(t, `
providers:
  - name: "shared"
    baseurl: "https://api.anthropic.com"
    apikey: "${TB_SHARED_KEY}"          # braced ref, shared across providers
    api_style: "anthropic"
    models: ["claude-3-5-sonnet-20241022"]
  - name: "openai"
    baseurl: "$TB_OPENAI_BASE"          # bare ref
    apikey: "${TB_SHARED_KEY}"
    api_style: "openai"
    models: ["gpt-4o"]
  - name: "unset"
    baseurl: "https://api.example.com"
    apikey: "${TB_DEFINITELY_UNSET_VAR}" # unset -> stays literal -> skipped later
    api_style: "openai"
    models: ["gpt-4o"]
`)
	entries, err := LoadProvidersConfig(p)
	if err != nil {
		t.Fatalf("LoadProvidersConfig: %v", err)
	}
	want := map[string]struct {
		key string
		url string
	}{
		"shared": {"sk-shared-xyz", "https://api.anthropic.com"},
		"openai": {"sk-shared-xyz", "https://api.openai.com"},
		// "unset" expands to the literal ${...} — single model -> entry name == provider name
		"unset": {"${TB_DEFINITELY_UNSET_VAR}", "https://api.example.com"},
	}
	got := map[string]RealModelEntry{}
	for _, e := range entries {
		got[e.Provider] = e
	}
	for name, w := range want {
		e, ok := got[name]
		if !ok {
			t.Fatalf("entry %q missing from results", name)
		}
		if e.APIKey != w.key {
			t.Errorf("entry %q apikey: got %q want %q", name, e.APIKey, w.key)
		}
		if e.BaseURL != w.url {
			t.Errorf("entry %q baseurl: got %q want %q", name, e.BaseURL, w.url)
		}
	}
}
