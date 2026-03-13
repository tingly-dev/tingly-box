package guardrails

import (
	"strings"
	"testing"
)

func TestContentCombinedTextIncludesCommand(t *testing.T) {
	content := Content{
		Text: "hello",
		Command: &Command{
			Name: "run",
			Arguments: map[string]interface{}{
				"path": "/tmp",
			},
		},
	}

	got := content.CombinedText()
	if got == "" {
		t.Fatalf("expected combined text")
	}
	if got == "hello" {
		t.Fatalf("expected command to be included")
	}
	if want := "command: run"; !strings.Contains(got, want) {
		t.Fatalf("expected %q in %q", want, got)
	}
}

func TestContentFilterTargets(t *testing.T) {
	content := Content{
		Text: "hello",
		Command: &Command{
			Name: "run",
		},
		Messages: []Message{
			{Role: "user", Content: "hi"},
		},
	}

	filtered := content.Filter([]ContentType{ContentTypeCommand})
	if filtered.Text != "" || len(filtered.Messages) != 0 || filtered.Command == nil {
		t.Fatalf("expected only command content to remain")
	}

	if content.HasAny([]ContentType{ContentTypeText}) == false {
		t.Fatalf("expected text to be detected")
	}
	if content.HasAny([]ContentType{ContentTypeCommand}) == false {
		t.Fatalf("expected command to be detected")
	}
}

func TestParseShellCommand(t *testing.T) {
	parsed := ParseShellCommand(`ls -la ~/.ssh | grep id_rsa > out.txt`)
	if parsed == nil {
		t.Fatalf("expected parsed shell command")
	}
	if len(parsed.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(parsed.Commands))
	}
	if parsed.Commands[0].Program != "ls" {
		t.Fatalf("expected first program ls, got %q", parsed.Commands[0].Program)
	}
	if parsed.Commands[1].Program != "grep" {
		t.Fatalf("expected second program grep, got %q", parsed.Commands[1].Program)
	}
	if len(parsed.Operators) != 1 || parsed.Operators[0] != "|" {
		t.Fatalf("expected pipeline operator, got %#v", parsed.Operators)
	}
	if len(parsed.Redirects) != 1 || parsed.Redirects[0].Target != "out.txt" {
		t.Fatalf("expected redirect target out.txt, got %#v", parsed.Redirects)
	}
}

func TestContentCombinedTextUsesShellParseForBash(t *testing.T) {
	content := Content{
		Command: &Command{
			Name: "bash",
			Arguments: map[string]interface{}{
				"command":     "ls -la ~/.ssh",
				"description": "List the ssh directory contents",
			},
		},
	}

	got := content.CombinedTextFor([]ContentType{ContentTypeCommand})
	if !strings.Contains(got, "normalized.kind: shell") {
		t.Fatalf("expected normalized shell kind in %q", got)
	}
	if !strings.Contains(got, "normalized.resources: ~/.ssh") {
		t.Fatalf("expected normalized resource in %q", got)
	}
	if !strings.Contains(got, "normalized.actions: read") {
		t.Fatalf("expected normalized action in %q", got)
	}
	if !strings.Contains(got, "normalized.terms: ls -la ~/.ssh") {
		t.Fatalf("expected normalized terms in %q", got)
	}
	if strings.Contains(got, "description") {
		t.Fatalf("did not expect non-shell description in %q", got)
	}
}
