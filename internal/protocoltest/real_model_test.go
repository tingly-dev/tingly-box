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

// The top-level `env:` table is the first lookup source for ${VAR}/$VAR refs,
// falling back to the process env. Env values may themselves be references.
func TestLoadProvidersConfigEnvTable(t *testing.T) {
	t.Setenv("TB_PROC_KEY", "from-process-env")

	p := writeProvidersYAML(t, `
env:
  SHARED_KEY: "sk-shared-from-table"        # literal value in the table
  DERIVED: "${SHARED_KEY}-suffix"            # env value referencing another env entry
  PROC_OVERRIDE: "from-table"                # table wins over process env
providers:
  - name: "uses-shared"
    baseurl: "https://a.test"
    apikey: "${SHARED_KEY}"                   # resolves from env table
    api_style: "anthropic"
    models: ["m"]
  - name: "uses-derived"
    baseurl: "https://b.test"
    apikey: "${DERIVED}"                      # resolves via env table self-expansion
    api_style: "anthropic"
    models: ["m"]
  - name: "uses-proc"
    baseurl: "https://c.test"
    apikey: "${TB_PROC_KEY}"                  # not in table -> falls back to process env
    api_style: "anthropic"
    models: ["m"]
  - name: "table-overrides-proc"
    baseurl: "https://d.test"
    apikey: "${PROC_OVERRIDE}"                # table value wins over process env
    api_style: "anthropic"
    models: ["m"]
  - name: "unset"
    baseurl: "https://e.test"
    apikey: "${NEITHER_TABLE_NOR_PROC}"       # unset everywhere -> stays literal
    api_style: "anthropic"
    models: ["m"]
`)
	// TB_PROC_KEY is set in process env; PROC_OVERRIDE is set in BOTH -> table wins.
	t.Setenv("PROC_OVERRIDE", "from-process-should-lose")

	entries, err := LoadProvidersConfig(p)
	if err != nil {
		t.Fatalf("LoadProvidersConfig: %v", err)
	}
	got := map[string]RealModelEntry{}
	for _, e := range entries {
		got[e.Provider] = e
	}
	want := map[string]string{ // provider -> expected apikey
		"uses-shared":          "sk-shared-from-table",
		"uses-derived":         "sk-shared-from-table-suffix",
		"uses-proc":            "from-process-env",
		"table-overrides-proc": "from-table",
		"unset":                "${NEITHER_TABLE_NOR_PROC}",
	}
	for name, w := range want {
		e, ok := got[name]
		if !ok {
			t.Fatalf("entry %q missing", name)
		}
		if e.APIKey != w {
			t.Errorf("entry %q apikey: got %q want %q", name, e.APIKey, w)
		}
	}
}

// A provider's optional `prompt` field is propagated to each expanded entry and
// env-expanded like apikey/baseurl. Providers without `prompt` get an empty
// entry.Prompt (caller falls back to the agent default).
func TestLoadProvidersConfigPromptField(t *testing.T) {
	t.Setenv("TB_PROMPT_VAR", "injected")
	p := writeProvidersYAML(t, `
providers:
  - name: "with-prompt"
    baseurl: "https://a.test"
    apikey: "sk-a"
    api_style: "anthropic"
    models: ["m1", "m2"]
    prompt: "summarize this: ${TB_PROMPT_VAR}"   # env-expanded
  - name: "no-prompt"
    baseurl: "https://b.test"
    apikey: "sk-b"
    api_style: "anthropic"
    models: ["m1"]
`)
	entries, err := LoadProvidersConfig(p)
	if err != nil {
		t.Fatalf("LoadProvidersConfig: %v", err)
	}
	got := map[string]string{} // entry name -> prompt
	for _, e := range entries {
		got[e.Name] = e.Prompt
	}
	if got["with-prompt-m1"] != "summarize this: injected" {
		t.Errorf("with-prompt-m1 prompt: got %q", got["with-prompt-m1"])
	}
	if got["with-prompt-m2"] != "summarize this: injected" {
		t.Errorf("with-prompt-m2 prompt: got %q", got["with-prompt-m2"])
	}
	if got["no-prompt"] != "" {
		t.Errorf("no-prompt should be empty, got %q", got["no-prompt"])
	}
}

// All entries expanded from one provider share that provider's name in
// RealModelEntry.Provider — which is what harness --filter matches on, so a
// multi-model provider is selected with a single filter value.
func TestExpandPreservesProviderNameForFilter(t *testing.T) {
	p := writeProvidersYAML(t, `
providers:
  - name: "anthropic"
    baseurl: "https://a.test"
    apikey: "sk-a"
    api_style: "anthropic"
    models: ["claude-3-5-sonnet-20241022", "claude-3-5-haiku-20241022"]
`)
	entries, err := LoadProvidersConfig(p)
	if err != nil {
		t.Fatalf("LoadProvidersConfig: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Provider != "anthropic" {
			t.Errorf("entry %q Provider: got %q, want %q (--filter matches this)", e.Name, e.Provider, "anthropic")
		}
	}
}

// enable: false skips a provider entirely; unset (nil) and enable: true keep it.
func TestLoadProvidersConfigEnableSkip(t *testing.T) {
	p := writeProvidersYAML(t, `
providers:
  - name: "on-default"
    baseurl: "https://a.test"
    apikey: "sk-a"
    api_style: "anthropic"
    models: ["m1"]
  - name: "explicit-on"
    baseurl: "https://b.test"
    apikey: "sk-b"
    api_style: "anthropic"
    models: ["m1"]
    enable: true
  - name: "off"
    baseurl: "https://c.test"
    apikey: "sk-c"
    api_style: "anthropic"
    models: ["m1"]
    enable: false
`)
	entries, err := LoadProvidersConfig(p)
	if err != nil {
		t.Fatalf("LoadProvidersConfig: %v", err)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.Provider] = true
	}
	if !seen["on-default"] {
		t.Error("on-default (unset) should be included")
	}
	if !seen["explicit-on"] {
		t.Error("explicit-on (enable: true) should be included")
	}
	if seen["off"] {
		t.Error("off (enable: false) should be skipped")
	}
}
// provider-level prompts are ignored. Only a CLI --prompt can override it
// (that override happens in the harness, not here). Without a top-level
// prompt, provider-level prompts still apply.
func TestLoadProvidersConfigTopLevelPromptLocks(t *testing.T) {
	p := writeProvidersYAML(t, `
prompt: "TOP_LEVEL_PROMPT"        # locks the prompt for every entry
providers:
  - name: "prov-override"
    baseurl: "https://a.test"
    apikey: "sk-a"
    api_style: "anthropic"
    models: ["m1"]
    prompt: "provider-level-ignored"   # ignored because top-level is set
  - name: "prov-none"
    baseurl: "https://b.test"
    apikey: "sk-b"
    api_style: "anthropic"
    models: ["m1"]
    # no provider prompt -> still gets the locked top-level value
`)
	entries, err := LoadProvidersConfig(p)
	if err != nil {
		t.Fatalf("LoadProvidersConfig: %v", err)
	}
	for _, e := range entries {
		if e.Prompt != "TOP_LEVEL_PROMPT" {
			t.Errorf("entry %q prompt: got %q, want %q (top-level should lock)", e.Name, e.Prompt, "TOP_LEVEL_PROMPT")
		}
	}
}
