//go:build e2e
// +build e2e

package protocoltest_test

import (
	"path/filepath"
	"runtime"
	"testing"

	pt "github.com/tingly-dev/tingly-box/internal/protocoltest"
)

// clientsDir resolves the tests/clients root relative to this source file so
// the e2e test finds the Node driver from any working directory.
func clientsDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve caller path")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "tests", "clients")
}

// TestHarness_Codex runs the single-hop matrix through the Codex subprocess
// driver — a faithful TS/Node port of the OpenAI Codex CLI's Responses-API
// client. It exercises the gateway's openai_responses source path with Codex's
// exact request shape (instructions, reasoning, include=reasoning.encrypted_content,
// store=false, prompt_cache_key, tool_choice=auto, the shell + apply_patch
// tools, and Codex's identity headers) and a Codex-style SSE accumulator.
// Non-Responses sources are visibly skipped (Supports is Responses-only).
//
// Requires node on PATH (the driver uses Node's built-in fetch — no npm deps):
//
//	go test -tags e2e ./internal/protocoltest/... -run TestHarness_Codex
func TestHarness_Codex(t *testing.T) {
	pt.DefaultMatrix().WithClient(pt.NewCodexClient(clientsDir(t))).Run(t)
}
