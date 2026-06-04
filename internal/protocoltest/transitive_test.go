//go:build e2e
// +build e2e

package protocoltest_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pt "github.com/tingly-dev/tingly-box/internal/protocoltest"
)

func TestTransitiveChains(t *testing.T) {
	m := pt.DefaultMatrix()
	chains := m.DefaultChains()
	assert.Greater(t, len(chains), 0, "expect at least one transitive chain")

	// Every chain's join point must be valid: first.Target == second.Source
	for _, c := range chains {
		assert.Equal(t, c.First.Target, c.Second.Source,
			"chain %s: first target must equal second source", c)
	}
}

func TestTransitive_NonStreaming(t *testing.T) {
	m := pt.DefaultMatrix()
	m.Streaming = []bool{false}
	m.RunTransitive(t)
}

func TestTransitive_Streaming(t *testing.T) {
	m := pt.DefaultMatrix()
	m.Streaming = []bool{true}
	m.RunTransitive(t)
}

func TestTransitive_TextOnly(t *testing.T) {
	m := pt.DefaultMatrix().OnlyScenarios("text")
	m.Streaming = []bool{false}
	m.RunTransitive(t)
}
