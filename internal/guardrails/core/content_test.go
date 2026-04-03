package core

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
