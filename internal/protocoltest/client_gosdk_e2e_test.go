//go:build e2e
// +build e2e

package protocoltest_test

import (
	"testing"

	pt "github.com/tingly-dev/tingly-box/internal/protocoltest"
)

// TestHarness_GoSDK runs the single-hop matrix through the official Go SDKs
// (anthropic-sdk-go, openai-go) instead of the raw HTTP client, so the
// gateway is validated against real client stacks: strict response
// unmarshaling, SSE event dispatch (which requires `event:` lines for
// Anthropic), and stream accumulation.
//
//	go test -tags e2e ./internal/protocoltest/... -run TestHarness_GoSDK
func TestHarness_GoSDK(t *testing.T) {
	pt.DefaultMatrix().WithClient(pt.NewGoSDKClient()).Run(t)
}
