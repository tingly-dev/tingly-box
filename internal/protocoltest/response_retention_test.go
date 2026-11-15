package protocoltest

import "testing"

func TestResponseRetention(t *testing.T) {
	for _, c := range responseRetentionCases() {
		t.Run(c.name, func(t *testing.T) { t.Parallel(); env := NewTestEnv(t); defer env.Close(); c.run(t, env) })
	}
}
