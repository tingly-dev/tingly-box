package command

import (
	"fmt"
	"os"
)

// isStdinTTY reports whether stdin is connected to a terminal. Every
// command that falls back to an interactive prompt when an argument is
// omitted must check this first — a bufio.Reader blocked on a redirected
// stdin (a script, CI, or piped input) never gets a real answer: it either
// hangs waiting for input that will never come, or fails with a bare
// "failed to read input: EOF" once the pipe closes.
func isStdinTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// requireTTY returns a consistent, actionable error when stdin isn't a
// TTY, and nil when it's safe to go ahead and prompt. hint is the specific
// way to unblock — an explicit argument to pass, or a flag that skips the
// prompt entirely — so the "can't prompt, here's what to do instead"
// framing lives in one place instead of being rebuilt (slightly
// differently worded each time) at every call site that falls back to an
// interactive read.
func requireTTY(hint string) error {
	if isStdinTTY() {
		return nil
	}
	return fmt.Errorf("no TTY to prompt; %s", hint)
}
