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

func TestExpandEnvRefs(t *testing.T) {
	t.Setenv("TB_TEST_KEY", "sk-shared-123")
	t.Setenv("TB_TEST_URL", "https://example.test")

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"braced", "${TB_TEST_KEY}", "sk-shared-123"},
		{"bare", "$TB_TEST_KEY", "sk-shared-123"},
		{"embedded braced", "bearer ${TB_TEST_KEY}", "bearer sk-shared-123"},
		{"embedded bare", "key=$TB_TEST_KEY end", "key=sk-shared-123 end"},
		{"multiple", "${TB_TEST_KEY}@${TB_TEST_URL}", "sk-shared-123@https://example.test"},
		{"ref plus literal suffix (baseurl style)", "${TB_TEST_URL}/v1/", "https://example.test/v1/"},
		{"literal host plus ref port", "https://host.test:${TB_TEST_PORT}", "https://host.test:${TB_TEST_PORT}"},
		{"no dollar", "plain-literal", "plain-literal"},
		{"unset leaves as-is braced", "${TB_TEST_UNSET}", "${TB_TEST_UNSET}"},
		{"unset leaves as-is bare", "$TB_TEST_UNSET", "$TB_TEST_UNSET"},
		{"dollar not a ref", "cost is $5 and $10", "cost is $5 and $10"},
		{"empty var", "$", "$"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := expandEnvRefs(c.in); got != c.want {
				t.Fatalf("expandEnvRefs(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
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
