package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	tomlpkg "github.com/pelletier/go-toml/v2"
)

func TestRewriteTinglyLoopback(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		changed bool
	}{
		{"http://127.0.0.1:12580/tingly/claude_code", "http://localhost:12580/tingly/claude_code", true},
		{"http://127.0.0.1:12580/tingly/codex", "http://localhost:12580/tingly/codex", true},
		{"https://127.0.0.1:12580/tingly/opencode", "https://localhost:12580/tingly/opencode", true},
		// Already localhost — unchanged
		{"http://localhost:12580/tingly/claude_code", "http://localhost:12580/tingly/claude_code", false},
		// Different path — not ours, leave alone
		{"http://127.0.0.1:7890", "http://127.0.0.1:7890", false},
		{"http://127.0.0.1:8080/v1/messages", "http://127.0.0.1:8080/v1/messages", false},
		// Different host — leave alone
		{"http://192.168.1.10:12580/tingly/claude_code", "http://192.168.1.10:12580/tingly/claude_code", false},
		// Garbage in — pass through, no rewrite
		{"not a url", "not a url", false},
	}
	for _, c := range cases {
		got, changed := rewriteTinglyLoopback(c.in)
		if got != c.want || changed != c.changed {
			t.Errorf("rewriteTinglyLoopback(%q) = (%q, %v), want (%q, %v)", c.in, got, changed, c.want, c.changed)
		}
	}
}

func TestRewriteLoopbackHost(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		changed bool
	}{
		{"http://127.0.0.1:12580/tingly/codex", "http://localhost:12580/tingly/codex", true},
		{"http://127.0.0.1:12580", "http://localhost:12580", true},
		{"http://localhost:12580/tingly/codex", "http://localhost:12580/tingly/codex", false},
		{"http://192.168.1.10:12580", "http://192.168.1.10:12580", false},
	}
	for _, c := range cases {
		got, changed := rewriteLoopbackHost(c.in)
		if got != c.want || changed != c.changed {
			t.Errorf("rewriteLoopbackHost(%q) = (%q, %v), want (%q, %v)", c.in, got, changed, c.want, c.changed)
		}
	}
}

func TestRewriteClaudeSettingsHost(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	initial := map[string]interface{}{
		"env": map[string]interface{}{
			"ANTHROPIC_BASE_URL":   "http://127.0.0.1:12580/tingly/claude_code",
			"ANTHROPIC_AUTH_TOKEN": "tok",
		},
		"statusLine": map[string]interface{}{"type": "command", "command": "x"},
	}
	data, _ := json.MarshalIndent(initial, "", "  ")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rewritten, ok := rewriteClaudeSettingsHost(path)
	if !ok || rewritten != path {
		t.Fatalf("expected rewrite at %s, got ok=%v path=%q", path, ok, rewritten)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	env := got["env"].(map[string]interface{})
	if env["ANTHROPIC_BASE_URL"] != "http://localhost:12580/tingly/claude_code" {
		t.Errorf("ANTHROPIC_BASE_URL = %v", env["ANTHROPIC_BASE_URL"])
	}
	// Unrelated keys preserved
	if env["ANTHROPIC_AUTH_TOKEN"] != "tok" {
		t.Errorf("token clobbered: %v", env["ANTHROPIC_AUTH_TOKEN"])
	}
	if _, ok := got["statusLine"]; !ok {
		t.Errorf("statusLine clobbered")
	}
}

func TestRewriteClaudeSettingsHost_NotTingly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	// User pointed Claude at a different upstream — must not be touched.
	initial := map[string]interface{}{
		"env": map[string]interface{}{
			"ANTHROPIC_BASE_URL": "http://127.0.0.1:8080/v1/messages",
		},
	}
	data, _ := json.MarshalIndent(initial, "", "  ")
	_ = os.WriteFile(path, data, 0644)

	if _, ok := rewriteClaudeSettingsHost(path); ok {
		t.Fatalf("must not rewrite non-tingly URL")
	}
}

func TestRewriteClaudeSettingsHost_Missing(t *testing.T) {
	if _, ok := rewriteClaudeSettingsHost(filepath.Join(t.TempDir(), "nope.json")); ok {
		t.Fatalf("missing file must be a no-op")
	}
}

func TestRewriteCodexConfigHost(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	initial := map[string]interface{}{
		"model":          "tingly-codex",
		"model_provider": "tingly-box",
		"model_providers": map[string]interface{}{
			"tingly-box": map[string]interface{}{
				"name":     "OpenAI using Tingly Box",
				"base_url": "http://127.0.0.1:12580/tingly/codex",
				"wire_api": "responses",
			},
			"openai": map[string]interface{}{ // unrelated provider, leave alone
				"base_url": "http://127.0.0.1:11434/v1",
			},
		},
	}
	data, _ := tomlpkg.Marshal(initial)
	_ = os.WriteFile(path, data, 0644)

	if _, ok := rewriteCodexConfigHost(path); !ok {
		t.Fatalf("expected rewrite")
	}

	raw, _ := os.ReadFile(path)
	var got map[string]interface{}
	_ = tomlpkg.Unmarshal(raw, &got)
	tb := got["model_providers"].(map[string]interface{})["tingly-box"].(map[string]interface{})
	if tb["base_url"] != "http://localhost:12580/tingly/codex" {
		t.Errorf("base_url = %v", tb["base_url"])
	}
	// Other provider untouched
	openai := got["model_providers"].(map[string]interface{})["openai"].(map[string]interface{})
	if openai["base_url"] != "http://127.0.0.1:11434/v1" {
		t.Errorf("other provider clobbered: %v", openai["base_url"])
	}
}

func TestRewriteOpenCodeConfigHost(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")

	initial := map[string]interface{}{
		"$schema": "https://opencode.ai/config.json",
		"provider": map[string]interface{}{
			"tingly-box": map[string]interface{}{
				"name": "tingly-box",
				"npm":  "@ai-sdk/anthropic",
				"options": map[string]interface{}{
					"baseURL": "http://127.0.0.1:12580/tingly/opencode",
					"apiKey":  "tok",
				},
			},
		},
	}
	data, _ := json.MarshalIndent(initial, "", "  ")
	_ = os.WriteFile(path, data, 0644)

	if _, ok := rewriteOpenCodeConfigHost(path); !ok {
		t.Fatalf("expected rewrite")
	}

	raw, _ := os.ReadFile(path)
	var got map[string]interface{}
	_ = json.Unmarshal(raw, &got)
	options := got["provider"].(map[string]interface{})["tingly-box"].(map[string]interface{})["options"].(map[string]interface{})
	if options["baseURL"] != "http://localhost:12580/tingly/opencode" {
		t.Errorf("baseURL = %v", options["baseURL"])
	}
	if options["apiKey"] != "tok" {
		t.Errorf("apiKey clobbered: %v", options["apiKey"])
	}
}
