package command

import (
	"strings"
	"testing"
)

// TestResolveTokenKind_NoTTYWithoutArg_ClearError mirrors the agent show
// fix: `token view` / `token refresh` with no kind argument and no TTY to
// prompt on must fail fast with an actionable message instead of falling
// into the bufio prompt and erroring on EOF.
func TestResolveTokenKind_NoTTYWithoutArg_ClearError(t *testing.T) {
	withNonTTYStdin(t)

	_, err := resolveTokenKind("")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no TTY") {
		t.Errorf("error = %q, want it to mention the missing TTY", err.Error())
	}
	if !strings.Contains(err.Error(), "token view auth") {
		t.Errorf("error = %q, want a usage example", err.Error())
	}
}

func TestResolveTokenKind_ExplicitArgsStillWork(t *testing.T) {
	for _, tc := range []struct {
		arg  string
		want TokenKind
	}{
		{"auth", tokenKindAuth},
		{"user", tokenKindAuth},
		{"model", tokenKindModel},
		{"MODEL", tokenKindModel},
	} {
		got, err := resolveTokenKind(tc.arg)
		if err != nil {
			t.Errorf("resolveTokenKind(%q): unexpected error: %v", tc.arg, err)
			continue
		}
		if got != tc.want {
			t.Errorf("resolveTokenKind(%q) = %q, want %q", tc.arg, got, tc.want)
		}
	}

	if _, err := resolveTokenKind("bogus"); err == nil {
		t.Error("resolveTokenKind(\"bogus\"): expected an error, got nil")
	}
}
