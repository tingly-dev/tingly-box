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
	assert.Len(t, m.Sources, 4)
	assert.Len(t, m.Targets, 5)
	assert.GreaterOrEqual(t, len(m.Scenarios), 6)
	assert.Len(t, m.Streaming, 2)
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
