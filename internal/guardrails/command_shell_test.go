package guardrails

import (
	"reflect"
	"testing"
)

func TestExtractShellCommandTextUsesShellKeys(t *testing.T) {
	t.Parallel()

	got, ok := extractShellCommandText("Bash", map[string]interface{}{
		"command":     "ls -la ~/.ssh",
		"raw_command": "should-not-win",
	})
	if !ok {
		t.Fatalf("expected shell command to be extracted")
	}
	if got != "ls -la ~/.ssh" {
		t.Fatalf("expected command field, got %q", got)
	}
}

func TestExtractShellCommandTextFallsBackToRawJSON(t *testing.T) {
	t.Parallel()

	got, ok := extractShellCommandText("Bash", map[string]interface{}{
		"_raw": `{}{"command":"cat ~/.ssh/config","description":"read ssh config"}`,
	})
	if !ok {
		t.Fatalf("expected raw fallback to recover command")
	}
	if got != "cat ~/.ssh/config" {
		t.Fatalf("unexpected recovered command: %q", got)
	}
}

func TestParseShellCommandHandlesQuotesAndOperators(t *testing.T) {
	t.Parallel()

	parsed := ParseShellCommand(`grep "ssh config" ~/.ssh/config && cat ~/.ssh/config`)
	if parsed == nil {
		t.Fatalf("expected parsed shell command")
	}
	if len(parsed.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(parsed.Commands))
	}
	if parsed.Commands[0].Program != "grep" {
		t.Fatalf("expected grep, got %q", parsed.Commands[0].Program)
	}
	wantArgs := []string{"ssh config", "~/.ssh/config"}
	if !reflect.DeepEqual(parsed.Commands[0].Args, wantArgs) {
		t.Fatalf("unexpected first command args: %#v", parsed.Commands[0].Args)
	}
	if len(parsed.Operators) != 1 || parsed.Operators[0] != "&&" {
		t.Fatalf("unexpected operators: %#v", parsed.Operators)
	}
}

func TestParseShellCommandHandlesRedirects(t *testing.T) {
	t.Parallel()

	parsed := ParseShellCommand(`cat ~/.ssh/config 2>> errors.log`)
	if parsed == nil {
		t.Fatalf("expected parsed shell command")
	}
	if len(parsed.Redirects) != 1 {
		t.Fatalf("expected one redirect, got %d", len(parsed.Redirects))
	}
	if parsed.Redirects[0].Op != "2>>" || parsed.Redirects[0].Target != "errors.log" {
		t.Fatalf("unexpected redirect: %#v", parsed.Redirects[0])
	}
}

func TestCommandAttachDerivedFieldsBuildsShellAndNormalizedViews(t *testing.T) {
	t.Parallel()

	cmd := &Command{
		Name: "bash",
		Arguments: map[string]interface{}{
			"command": "ls -la ~/.ssh | grep config",
		},
	}

	cmd.AttachDerivedFields()

	if cmd.Shell == nil {
		t.Fatalf("expected parsed shell view")
	}
	if cmd.Normalized == nil {
		t.Fatalf("expected normalized command view")
	}
	if cmd.Normalized.Kind != "shell" {
		t.Fatalf("expected shell kind, got %q", cmd.Normalized.Kind)
	}
	if !reflect.DeepEqual(cmd.Normalized.Actions, []string{"read"}) {
		t.Fatalf("unexpected normalized actions: %#v", cmd.Normalized.Actions)
	}
	if !reflect.DeepEqual(cmd.Normalized.Resources, []string{"~/.ssh"}) {
		t.Fatalf("unexpected normalized resources: %#v", cmd.Normalized.Resources)
	}
}
