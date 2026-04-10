package protocol_validate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pv "github.com/tingly-dev/tingly-box/internal/protocol_validate"
)

// TestDefaultMatrix verifies the default matrix covers all expected protocol combinations.
func TestDefaultMatrix(t *testing.T) {
	m := pv.DefaultMatrix()
	require.NotNil(t, m)

	// All 4 source protocols
	assert.Len(t, m.Sources, 4)

	// All 5 target protocols
	assert.Len(t, m.Targets, 5)

	// At least the 6 baseline scenarios
	assert.GreaterOrEqual(t, len(m.Scenarios), 6)

	// Both streaming modes
	assert.Len(t, m.Streaming, 2)
}

// TestMatrix_Run_NonStreaming runs the full non-streaming matrix.
// Each scenario × source × target is tested with a virtual provider.
func TestMatrix_Run_NonStreaming(t *testing.T) {
	m := pv.DefaultMatrix()
	m.Streaming = []bool{false} // non-streaming only
	m.Run(t)
}

// TestMatrix_Run_Streaming runs the full streaming matrix.
func TestMatrix_Run_Streaming(t *testing.T) {
	m := pv.DefaultMatrix()
	m.Streaming = []bool{true} // streaming only
	m.Run(t)
}

// TestMatrix_FilterByScenario verifies scenario filtering works correctly.
func TestMatrix_FilterByScenario(t *testing.T) {
	m := pv.DefaultMatrix().OnlyScenarios("text", "tool_use")
	assert.Len(t, m.Scenarios, 2)
	assert.Equal(t, "text", m.Scenarios[0].Name)
	assert.Equal(t, "tool_use", m.Scenarios[1].Name)
}

// TestMatrix_SkipUnsupported verifies skip rules prevent test execution for known gaps.
func TestMatrix_SkipUnsupported(t *testing.T) {
	m := pv.DefaultMatrix().OnlyScenarios("text")
	m.Streaming = []bool{false}
	// Skips should be registered for unsupported source→target pairs.
	// The matrix runner must not fail for those — it must skip them.
	m.Run(t)
}
