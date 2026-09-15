package command

import (
	"strings"
	"testing"
)

func TestResolveInstallPackage_DefaultsByAgent(t *testing.T) {
	cases := []struct{ agent, want string }{
		{"cc", "@anthropic-ai/claude-code"},
		{"claude-code", "@anthropic-ai/claude-code"},
		{"oc", "opencode-ai"},
		{"opencode", "opencode-ai"},
		{"cx", "@openai/codex"},
		{"codex", "@openai/codex"},
	}
	for _, c := range cases {
		got, err := resolveInstallPackage(c.agent, "", "")
		if err != nil {
			t.Errorf("resolveInstallPackage(%q): %v", c.agent, err)
			continue
		}
		if got != c.want {
			t.Errorf("resolveInstallPackage(%q) = %q, want %q", c.agent, got, c.want)
		}
	}
}

func TestResolveInstallPackage_VersionAppended(t *testing.T) {
	got, err := resolveInstallPackage("cc", "", "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if got != "@anthropic-ai/claude-code@1.2.3" {
		t.Errorf("got %q", got)
	}
}

func TestResolveInstallPackage_OverrideTakesPrecedence(t *testing.T) {
	got, err := resolveInstallPackage("cc", "my-fork-of-cc", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "my-fork-of-cc" {
		t.Errorf("got %q, want my-fork-of-cc", got)
	}

	// Override + version combine.
	got, err = resolveInstallPackage("cc", "my-fork-of-cc", "0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if got != "my-fork-of-cc@0.1.0" {
		t.Errorf("got %q", got)
	}
}

func TestResolveInstallPackage_MissingAgent(t *testing.T) {
	_, err := resolveInstallPackage("", "", "")
	if err == nil || !strings.Contains(err.Error(), "--agent") {
		t.Errorf("expected --agent error, got %v", err)
	}
}

func TestResolveInstallPackage_InvalidAgent(t *testing.T) {
	_, err := resolveInstallPackage("nope", "", "")
	if err == nil {
		t.Fatal("expected error for invalid agent")
	}
}

func TestResolveInstallPackage_TrimsWhitespace(t *testing.T) {
	got, err := resolveInstallPackage("cc", "  ", "  ")
	if err != nil {
		t.Fatal(err)
	}
	// Whitespace-only override / version are equivalent to absent.
	if got != "@anthropic-ai/claude-code" {
		t.Errorf("got %q", got)
	}
}
