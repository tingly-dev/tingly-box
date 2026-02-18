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
