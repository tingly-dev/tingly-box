//go:build !windows

package runtime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestRuntime_Close_HungStdioSource is a regression test for `tb restart`/
// `stop` hanging (or leaking a subprocess) when a configured stdio MCP
// server doesn't shut down cleanly.
//
// The underlying SDK's CommandTransport.Close does not honor the context
// passed to it, so a source that ignores SIGTERM and never reacts to its
// stdin closing can only be ended by SIGKILL. Runtime.Close bounds the
// whole disconnect at a shared timeout and force-kills whatever's left
// (see ForceKill) rather than waiting on — or abandoning — that source
// indefinitely. This test connects one such source (testdata/fakemcp,
// FAKE_MCP_SLOW=1) and asserts Close returns promptly and the subprocess is
// actually dead afterward, not merely orphaned.
func TestRuntime_Close_HungStdioSource(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a real subprocess; skipped in -short")
	}

	fakemcpPath := buildFakeMCP(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sc := newSessionCache()
	src, err := NewStdioToolSource(typ.MCPSourceConfig{
		ID:        "hung",
		Name:      "hung",
		Transport: "stdio",
		Command:   fakemcpPath,
		Enabled:   typ.BoolPtr(true),
		Env:       map[string]string{"FAKE_MCP_SLOW": "1"},
	}, sc)
	if err != nil {
		t.Fatalf("failed to create stdio source: %v", err)
	}
	if err := src.Connect(ctx); err != nil {
		t.Fatalf("failed to connect stdio source: %v", err)
	}

	r := &Runtime{
		sc:            sc,
		activeSources: map[string]ToolSource{"hung": src},
	}

	closeDone := make(chan struct{})
	start := time.Now()
	go func() {
		r.Close()
		close(closeDone)
	}()

	const closeDeadline = 7 * time.Second // Close's own budget is 5s; leave margin.
	select {
	case <-closeDone:
	case <-time.After(closeDeadline):
		t.Fatalf("Runtime.Close() did not return within %v for a hung MCP source — this is exactly what made `tb restart`/`stop` hang", closeDeadline)
	}
	t.Logf("Close() returned in %v", time.Since(start).Round(time.Millisecond))

	if cmd := src.killCmd.Load(); cmd != nil && cmd.Process != nil {
		// A killed-but-not-yet-reaped process still answers signal 0.
		if err := cmd.Process.Signal(syscall.Signal(0)); err == nil {
			t.Error("hung MCP subprocess is still alive after Close() returned — it was orphaned instead of force-killed")
		}
	}
}

// buildFakeMCP compiles the fake MCP stdio server under testdata/fakemcp
// into a temp binary and returns its path.
func buildFakeMCP(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Skipf("cannot determine working directory: %v", err)
	}
	pkgDir := filepath.Join(wd, "testdata", "fakemcp")
	if _, err := os.Stat(pkgDir); err != nil {
		t.Skipf("fakemcp source not found at %s: %v", pkgDir, err)
	}

	bin := filepath.Join(t.TempDir(), "fakemcp")
	cmd := exec.Command("go", "build", "-o", bin, "./testdata/fakemcp")
	cmd.Dir = wd
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("failed to build fakemcp: %v\n%s", err, out)
	}

	t.Cleanup(func() {
		// Best-effort: the whole point of this test is that Close() force
		// kills the subprocess, but guard against a regression leaving one
		// behind anyway.
		_ = exec.Command("pkill", "-9", "-f", bin).Run()
	})

	return bin
}
