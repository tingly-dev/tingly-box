package main

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The per-entry timeout is passed in, not read from process-wide state.
func TestRunAgentCmdWithTimeout(t *testing.T) {
	t.Run("timeout kills the process", func(t *testing.T) {
		start := time.Now()
		out, timedOut, err := runAgentCmdWithTimeout(exec.Command("sleep", "5"), 50*time.Millisecond)
		if !timedOut || err == nil {
			t.Fatalf("timedOut=%v err=%v out=%q", timedOut, err, out)
		}
		if time.Since(start) > 3*time.Second {
			t.Fatalf("sleep was not killed promptly: %v", time.Since(start))
		}
	})
	t.Run("zero disables the timeout", func(t *testing.T) {
		out, timedOut, err := runAgentCmdWithTimeout(exec.Command("echo", "hi"), 0)
		if timedOut || err != nil || strings.TrimSpace(string(out)) != "hi" {
			t.Fatalf("timedOut=%v err=%v out=%q", timedOut, err, out)
		}
	})
}

func TestFinishAgentResultClassifiesOutcome(t *testing.T) {
	start := time.Now()
	r := &AgentTestResult{}
	finishAgentResult(r, start, []byte("x"), true, 2*time.Second, nil)
	if !r.TimedOut || r.Success || r.ExitCode != -1 || r.Error != "timeout after 2s" {
		t.Fatalf("timeout: %+v", r)
	}
	r = &AgentTestResult{}
	finishAgentResult(r, start, []byte("ok"), false, time.Second, nil)
	if !r.Success || r.Output != "ok" {
		t.Fatalf("success: %+v", r)
	}
}

// Killing the agent CLI must bound the run even when a descendant (a runtime
// launcher, a spawned tool) inherited the output pipe and outlives it: the
// shell below is killed at the timeout but its `sleep` keeps the pipe open.
func TestRunAgentCmdWithTimeoutDescendantHoldsPipe(t *testing.T) {
	old := agentKillGrace
	agentKillGrace = 200 * time.Millisecond
	defer func() { agentKillGrace = old }()

	start := time.Now()
	out, timedOut, err := runAgentCmdWithTimeout(exec.Command("sh", "-c", "echo started; sleep 5"), 100*time.Millisecond)
	elapsed := time.Since(start)
	if !timedOut || err == nil {
		t.Fatalf("timedOut=%v err=%v", timedOut, err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("returned after %v, want shortly after the %v timeout + %v grace", elapsed, 100*time.Millisecond, agentKillGrace)
	}
	if !strings.Contains(string(out), "started") {
		t.Fatalf("output captured before the kill was lost: %q", out)
	}
}
