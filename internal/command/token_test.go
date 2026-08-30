package command

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout runs fn with os.Stdout redirected to a pipe and returns
// everything written to it.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	os.Stdout = w
	out := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		out <- buf.String()
	}()
	fn()
	_ = w.Close()
	os.Stdout = oldStdout
	return <-out
}

// TestTokenViewCmdKong_NoKind_ShowsBothWithoutPrompting covers the
// requested behavior change: `token view` with no kind argument used to
// prompt "Which tingly-box token?" (reading from stdin); it must now print
// both tokens directly, with no prompt and no stdin read at all — it works
// the same whether or not a TTY is attached.
func TestTokenViewCmdKong_NoKind_ShowsBothWithoutPrompting(t *testing.T) {
	withNonTTYStdin(t) // no TTY: if this fell back to the old prompt path, it would error out instead of succeeding
	am := newTestAppManager(t)

	var err error
	output := captureStdout(t, func() {
		err = (&TokenViewCmdKong{}).Run(am)
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(output, "Which tingly-box token?") {
		t.Error("output still contains the old choose-one prompt")
	}
	if !strings.Contains(output, "Kind:     auth") {
		t.Errorf("output missing auth token section:\n%s", output)
	}
	if !strings.Contains(output, "Kind:     model") {
		t.Errorf("output missing model token section:\n%s", output)
	}
}

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
