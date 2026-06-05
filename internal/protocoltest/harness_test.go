//go:build e2e
// +build e2e

package protocoltest_test

import (
	"testing"

	pt "github.com/tingly-dev/tingly-box/internal/protocoltest"
)

// TestHarness is the primary entry point for the full protocol validation
// harness. It runs two sections:
//
//	single_hop — every (source→target) pair × scenario × streaming mode
//	two_hop    — every (A→B→C) transitive chain × scenario × streaming mode
//
// Selective execution:
//
//	go test -tags e2e ./internal/protocoltest/... -run TestHarness
//	go test -tags e2e ./internal/protocoltest/... -run TestHarness/single_hop
//	go test -tags e2e ./internal/protocoltest/... -run TestHarness/two_hop
func TestHarness(t *testing.T) {
	pt.DefaultMatrix().RunFull(t)
}

// TestHarness_NonStreaming runs both sections in non-streaming mode only.
func TestHarness_NonStreaming(t *testing.T) {
	m := pt.DefaultMatrix()
	m.Streaming = []bool{false}
	m.RunFull(t)
}

// TestHarness_Streaming runs both sections in streaming mode only.
func TestHarness_Streaming(t *testing.T) {
	m := pt.DefaultMatrix()
	m.Streaming = []bool{true}
	m.RunFull(t)
}
