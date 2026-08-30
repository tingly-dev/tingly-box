package command

import (
	"strings"
	"testing"
)

// TestOAuthCmdKong_NoProvider_NoTTY_ClearError guards the no-TTY crash: with
// no provider argument, Run used to fall straight into runInteractiveMode's
// bufio prompt, which failed with a bare "failed to read input: EOF" once
// stdin wasn't a TTY (e.g. piped input, CI). It must fail with a clear,
// actionable error instead.
func TestOAuthCmdKong_NoProvider_NoTTY_ClearError(t *testing.T) {
	withNonTTYStdin(t)

	am := newTestAppManager(t)
	err := (&OAuthCmdKong{}).Run(am)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no TTY") {
		t.Errorf("error = %q, want it to mention the missing TTY", err.Error())
	}
}
