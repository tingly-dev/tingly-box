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
