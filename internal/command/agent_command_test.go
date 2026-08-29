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

// TestAgentApplyFlagCmdKong_NoTTYWithoutAgentType_ClearError is the same
// regression guard as the show/token-view fix, for `agent apply`'s own
// agent-type prompt.
func TestAgentApplyFlagCmdKong_NoTTYWithoutAgentType_ClearError(t *testing.T) {
	withNonTTYStdin(t)

	err := (&AgentApplyFlagCmdKong{}).Run(nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no TTY") {
		t.Errorf("error = %q, want it to mention the missing TTY", err.Error())
	}
	if !strings.Contains(err.Error(), "agent apply claude-code") {
		t.Errorf("error = %q, want a usage example", err.Error())
	}
}

// TestAgentApplyFlagCmdKong_NoTTYWithoutForce_ClearError: `apply` is meant
// to be a one-shot "configure and go" command (unlike show/restore, it's
// the one CLI verb genuinely worth running non-interactively often), so a
// missing confirmation TTY must fail with a hint to pass --force rather
// than block on a bufio read that can never succeed. Needs a real
// AppManager: with an agent type given, Run reaches routing-rule
// resolution (which needs a config) before the force/TTY check.
func TestAgentApplyFlagCmdKong_NoTTYWithoutForce_ClearError(t *testing.T) {
	withNonTTYStdin(t)
	am := newTestAppManager(t)

	var err error
	withSilencedStdout(t, func() {
		err = (&AgentApplyFlagCmdKong{AgentType: "claude-code"}).Run(am)
	})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no TTY") || !strings.Contains(err.Error(), "--force") {
		t.Errorf("error = %q, want it to mention the missing TTY and --force", err.Error())
	}
}
