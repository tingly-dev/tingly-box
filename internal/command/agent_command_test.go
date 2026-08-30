package command

import (
	"os"
	"strings"
	"testing"
)

// withNonTTYStdin temporarily replaces os.Stdin with the read end of a pipe,
// which os.ModeCharDevice never reports for. This is enough to make
// isStdinTTY() return false without needing a real pseudo-terminal.
func withNonTTYStdin(t *testing.T) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	original := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = original })
}

// TestAgentShowFlagCmdKong_NoTTYWithoutAgentType_ClearError guards against a
// regression to the old behavior: with no agent type given and no TTY to
// prompt on, `agent show` used to fall into promptForAgentTypeChoice and
// crash with a bare "failed to read input: EOF" once the bufio.Reader hit
// end of input. It must instead fail fast with an actionable message and
// never touch appManager (passed as nil here) before returning.
func TestAgentShowFlagCmdKong_NoTTYWithoutAgentType_ClearError(t *testing.T) {
	withNonTTYStdin(t)

	err := (&AgentShowFlagCmdKong{}).Run(nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no TTY") {
		t.Errorf("error = %q, want it to mention the missing TTY", err.Error())
	}
	if !strings.Contains(err.Error(), "agent show claude-code") {
		t.Errorf("error = %q, want a usage example", err.Error())
	}
}

// TestAgentApplyFlagCmdKong_WithoutAgentType_ClearError: unlike agent
// show/restore, apply never falls back to an interactive picker at all —
// it's a one-shot "apply the defaults" command, not a wizard, so a missing
// agent type is always a clear error, TTY or not (a TTY doesn't change
// whether picking interactively is the right thing for apply to do).
func TestAgentApplyFlagCmdKong_WithoutAgentType_ClearError(t *testing.T) {
	err := (&AgentApplyFlagCmdKong{}).Run(nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "agent type required") {
		t.Errorf("error = %q, want it to say an agent type is required", err.Error())
	}
	if !strings.Contains(err.Error(), "agent apply claude-code") {
		t.Errorf("error = %q, want a usage example", err.Error())
	}
}

// TestAgentApplyFlagCmdKong_NoTTYWithoutYes_ClearError: `apply` is meant
// to be a one-shot "configure and go" command (unlike show/restore, it's
// the one CLI verb genuinely worth running non-interactively often), so a
// missing confirmation TTY must fail with a hint to pass -y/--yes rather
// than block on a bufio read that can never succeed. Needs a real
// AppManager: with an agent type given, Run reaches routing-rule
// resolution (which needs a config) before the yes/TTY check.
func TestAgentApplyFlagCmdKong_NoTTYWithoutYes_ClearError(t *testing.T) {
	withNonTTYStdin(t)
	am := newTestAppManager(t)

	var err error
	withSilencedStdout(t, func() {
		err = (&AgentApplyFlagCmdKong{AgentType: "claude-code"}).Run(am)
	})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no TTY") || !strings.Contains(err.Error(), "-y/--yes") {
		t.Errorf("error = %q, want it to mention the missing TTY and -y/--yes", err.Error())
	}
}

// TestAgentApplyFlagCmdKong_NoRoutingRule_NeverPromptsForProvider is the
// core regression guard for the redesign: apply is a one-shot "apply the
// defaults" command, so with no routing rule configured it must proceed
// with config-files-only rather than falling back to an interactive
// provider/model picker (the old promptForAgentConfig path, now removed
// entirely). Preview mode returns before the confirm step, so this also
// proves the picker isn't reachable earlier in Run, without needing
// -y/--yes or touching real Claude Code config files.
func TestAgentApplyFlagCmdKong_NoRoutingRule_NeverPromptsForProvider(t *testing.T) {
	withNonTTYStdin(t) // a fallback to the old picker would block reading this
	am := newTestAppManager(t)

	var err error
	output := captureStdout(t, func() {
		err = (&AgentApplyFlagCmdKong{AgentType: "claude-code", Preview: true}).Run(am)
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(output, "no service configured") {
		t.Errorf("output = %q, want the config-files-only preview note", output)
	}
}
