//go:build e2e
// +build e2e

package protocol_validate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pt "github.com/tingly-dev/tingly-box/internal/protocol_validate"
)

func TestDefaultMatrix(t *testing.T) {
	m := pt.DefaultMatrix()
	require.NotNil(t, m)
	assert.GreaterOrEqual(t, len(m.Pairs), 12, "expect every source to have at least one pair")
	assert.GreaterOrEqual(t, len(m.Scenarios), 6)
	assert.Len(t, m.Streaming, 2)

	// Every pair should be distinct and source/target non-empty.
	seen := make(map[pt.ProtocolPair]bool, len(m.Pairs))
	for _, p := range m.Pairs {
		assert.NotEmpty(t, p.Source, "pair source must be set")
		assert.NotEmpty(t, p.Target, "pair target must be set")
		assert.False(t, seen[p], "duplicate pair: %s", p)
		seen[p] = true
	}
}

func TestMatrix_Run_NonStreaming(t *testing.T) {
	m := pt.DefaultMatrix()
	m.Streaming = []bool{false}
	m.Run(t)
}

func TestMatrix_Run_Streaming(t *testing.T) {
	m := pt.DefaultMatrix()
	m.Streaming = []bool{true}
	m.Run(t)
}

func TestMatrix_FilterByScenario(t *testing.T) {
	m := pt.DefaultMatrix().OnlyScenarios("text", "tool_use")
	assert.Len(t, m.Scenarios, 2)
	assert.Equal(t, "text", m.Scenarios[0].Name)
	assert.Equal(t, "tool_use", m.Scenarios[1].Name)
}

func TestMatrix_SkipUnsupported(t *testing.T) {
	m := pt.DefaultMatrix().OnlyScenarios("text")
	m.Streaming = []bool{false}
	m.Run(t)
}
