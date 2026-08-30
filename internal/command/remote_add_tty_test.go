package command

import (
	"strings"
	"testing"
)

// TestRemoteAdd_NoTTY_ClearError guards the legacy bufio interactive
// fallback documented as transitional in .design/tui.md §12. remote add has
// no flag form at all — before this check it would hang or crash with a
// bare "failed to read input: EOF" without a TTY. Unlike provider/rule,
// this command genuinely has no non-interactive alternative to fall back
// to, so a TTY check (rather than a flag-driven CI form) is the fix.
func TestRemoteAdd_NoTTY_ClearError(t *testing.T) {
	withNonTTYStdin(t)

	err := (&RemoteAddCmdKong{}).Run(nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no TTY") {
		t.Errorf("error = %q, want it to mention the missing TTY", err.Error())
	}
}
