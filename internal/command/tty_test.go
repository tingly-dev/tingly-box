package command

import (
	"strings"
	"testing"
)

// TestRequireTTY covers the shared helper directly: every command's
// "no TTY, here's how to unblock" gate goes through this one function
// (see tty.go) instead of hand-rolling the check and message each time.
func TestRequireTTY(t *testing.T) {
	t.Run("no TTY", func(t *testing.T) {
		withNonTTYStdin(t)

		err := requireTTY("pass it explicitly, e.g. 'foo bar'")
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "no TTY to prompt") {
			t.Errorf("error = %q, want it to lead with the shared framing", err.Error())
		}
		if !strings.Contains(err.Error(), "pass it explicitly, e.g. 'foo bar'") {
			t.Errorf("error = %q, want it to carry the caller's hint", err.Error())
		}
	})
}
