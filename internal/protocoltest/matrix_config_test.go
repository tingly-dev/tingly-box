package protocoltest_test

// Configuration-level assertions for the harness matrix. Full matrix
// execution lives in cli/harness (CI: harness-matrix.yml); these tests only
// guard the matrix definition itself — pair/scenario/chain shape — so they
// run in the default suite without booting any gateway.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pt "github.com/tingly-dev/tingly-box/internal/protocoltest"
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

func TestMatrix_FilterByScenario(t *testing.T) {
	m := pt.DefaultMatrix().OnlyScenarios("text", "tool_use")
	assert.Len(t, m.Scenarios, 2)
	assert.Equal(t, "text", m.Scenarios[0].Name)
	assert.Equal(t, "tool_use", m.Scenarios[1].Name)
}

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

func TestIdempotentCases_Defined(t *testing.T) {
	cases := pt.DefaultIdempotentCases()
	assert.Greater(t, len(cases), 0, "expect at least one idempotency case")
	for _, c := range cases {
		assert.NotEmpty(t, c.Source, "case %s: source must be set", c.Name)
		assert.NotEmpty(t, c.Mid, "case %s: mid must be set", c.Name)
		assert.NotEmpty(t, c.Baseline, "case %s: baseline must be set", c.Name)
		assert.NotEqual(t, c.Source, c.Mid, "case %s: source and mid must differ", c.Name)
	}
}
