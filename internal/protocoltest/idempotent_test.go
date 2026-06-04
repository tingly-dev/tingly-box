//go:build e2e
// +build e2e

package protocoltest_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pt "github.com/tingly-dev/tingly-box/internal/protocoltest"
)

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

func TestIdempotent_NonStreaming(t *testing.T) {
	m := pt.DefaultMatrix()
	m.Streaming = []bool{false}
	m.RunIdempotent(t)
}

func TestIdempotent_Streaming(t *testing.T) {
	m := pt.DefaultMatrix()
	m.Streaming = []bool{true}
	m.RunIdempotent(t)
}

func TestIdempotent_TextOnly(t *testing.T) {
	m := pt.DefaultMatrix().OnlyScenarios("text")
	m.Streaming = []bool{false}
	m.RunIdempotent(t)
}
