//go:build experiment

package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestRestartLatencyExperiment is a kept, re-runnable experiment (not a
// pass/fail correctness check) investigating why `tb restart` could be very
// slow. It measures, with real subprocesses, how long shutting down N
// configured stdio MCP sources actually takes, factored across:
//
//   - number of configured sources (1 vs 3)
//   - whether the subprocess behaves (exits as soon as its stdin closes,
//     per the MCP stdio shutdown spec) or is unresponsive (ignores SIGTERM
//     and never reacts to stdin closing — needs SIGKILL)
//
// against the two candidate implementations of Runtime.Close():
//
//   - "sequential" — the pre-fix behavior: disconnect sources one at a time
//     under a single nominal 5s context that the underlying SDK transport
//     does not actually honor (see mcp.CommandTransport / pipeRWC.Close in
//     the modelcontextprotocol/go-sdk dependency), so each unresponsive
//     source adds its own ~10s wait (5s idle + 5s after SIGTERM before
//     SIGKILL) on top of the others, sequentially.
//   - "concurrent" — the current internal/mcp/runtime/runtime.go Close():
//     disconnect all sources at once and return once a single shared 5s
//     budget elapses, regardless of source count.
//
// Gated behind the "experiment" build tag (it forks real subprocesses and
// takes about a minute) so it never runs as part of `go test ./internal/...`
// in CI. Run it directly to see current numbers on this machine:
//
//	go test -tags experiment ./internal/mcp/runtime/ -run TestRestartLatencyExperiment -v
//
// See scripts/experiments/restart-latency/README.md for the fuller
// background on the bug this reproduces and a sample captured run.
func TestRestartLatencyExperiment(t *testing.T) {
	if testing.Short() {
		t.Skip("restart-latency experiment is slow (~1 min); skipped in -short")
	}

	fakemcpPath := buildFakeMCP(t)

	type scenario struct {
		sources int
		slow    bool
	}
	scenarios := []scenario{
		{sources: 1, slow: false},
		{sources: 1, slow: true},
		{sources: 3, slow: false},
		{sources: 3, slow: true},
	}

	type result struct {
		scenario   scenario
		sequential time.Duration
		concurrent time.Duration
	}
	var results []result

	for _, sc := range scenarios {
		sc := sc
		name := fmt.Sprintf("sources=%d/slow=%v", sc.sources, sc.slow)
		t.Run(name, func(t *testing.T) {
			seq := timeSequentialClose(t, fakemcpPath, sc.sources, sc.slow)
			t.Logf("sequential (pre-fix) close:   %v", seq.Round(time.Millisecond))

			conc := timeConcurrentClose(t, fakemcpPath, sc.sources, sc.slow)
			t.Logf("concurrent (current fix) close: %v", conc.Round(time.Millisecond))

			results = append(results, result{scenario: sc, sequential: seq, concurrent: conc})
		})
	}

	t.Log("=== restart-latency experiment summary (this machine, this run) ===")
	t.Logf("%-20s %-14s %-14s", "scenario", "sequential", "concurrent")
	for _, r := range results {
		t.Logf("%-20s %-14v %-14v",
			fmt.Sprintf("sources=%d/slow=%v", r.scenario.sources, r.scenario.slow),
			r.sequential.Round(time.Millisecond), r.concurrent.Round(time.Millisecond))
	}
}

// buildFakeMCP compiles the experiment's fake MCP stdio server (see
// scripts/experiments/restart-latency/fakemcp) into a temp binary and
// returns its path. It skips the test if the source tree isn't reachable
// (e.g. this package is vendored/extracted on its own).
func buildFakeMCP(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Skipf("cannot determine working directory: %v", err)
	}
	// internal/mcp/runtime -> project root
	projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(wd)))
	pkgDir := filepath.Join(projectRoot, "scripts", "experiments", "restart-latency", "fakemcp")
	if _, err := os.Stat(pkgDir); err != nil {
		t.Skipf("fakemcp source not found at %s: %v", pkgDir, err)
	}

	bin := filepath.Join(t.TempDir(), "fakemcp")
	cmd := exec.Command("go", "build", "-o", bin, "./scripts/experiments/restart-latency/fakemcp")
	cmd.Dir = projectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("failed to build fakemcp: %v\n%s", err, out)
	}

	t.Cleanup(func() {
		// Best-effort: reap any fakemcp subprocess a "concurrent close"
		// scenario deliberately left running past its own return — the
		// whole point of that scenario is that Close() does NOT wait for
		// stragglers, so one may still be mid-SIGKILL-escalation when the
		// test function itself moves on.
		_ = exec.Command("pkill", "-9", "-f", bin).Run()
	})

	return bin
}

func newFakeSourceConfig(id, fakemcpPath string, slow bool) typ.MCPSourceConfig {
	cfg := typ.MCPSourceConfig{
		ID:        id,
		Name:      id,
		Transport: "stdio",
		Command:   fakemcpPath,
		Enabled:   typ.BoolPtr(true),
	}
	if slow {
		cfg.Env = map[string]string{"FAKE_MCP_SLOW": "1"}
	}
	return cfg
}

// connectedSources creates and connects n independent stdio sources, each
// with its own session cache and subprocess, so every timed phase starts
// from a completely fresh set of live connections.
func connectedSources(t *testing.T, fakemcpPath string, n int, slow bool) map[string]ToolSource {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sources := make(map[string]ToolSource, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("fake-%d", i)
		sc := newSessionCache()
		src, err := NewStdioToolSource(newFakeSourceConfig(id, fakemcpPath, slow), sc)
		if err != nil {
			t.Fatalf("failed to create stdio source: %v", err)
		}
		if err := src.Connect(ctx); err != nil {
			t.Fatalf("failed to connect stdio source %d: %v", i, err)
		}
		sources[id] = src
	}
	return sources
}

// timeSequentialClose reproduces the pre-fix Runtime.Close(): sources are
// disconnected one at a time under a single ctx that the SDK's stdio
// transport does not actually honor (see mcp.CommandTransport.Connect /
// pipeRWC.Close), so the wall-clock cost is the sum of each source's own
// shutdown time rather than a shared bound.
func timeSequentialClose(t *testing.T, fakemcpPath string, n int, slow bool) time.Duration {
	t.Helper()
	sources := connectedSources(t, fakemcpPath, n, slow)

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for id, source := range sources {
		if err := source.Disconnect(ctx); err != nil {
			t.Logf("disconnect %s: %v", id, err)
		}
	}
	return time.Since(start)
}

// timeConcurrentClose exercises the actual, current Runtime.Close().
func timeConcurrentClose(t *testing.T, fakemcpPath string, n int, slow bool) time.Duration {
	t.Helper()
	sources := connectedSources(t, fakemcpPath, n, slow)

	r := &Runtime{
		sc:            newSessionCache(),
		activeSources: sources,
	}

	start := time.Now()
	r.Close()
	return time.Since(start)
}
