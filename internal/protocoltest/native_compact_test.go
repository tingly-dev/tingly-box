package protocoltest

import "testing"

func TestNativeCompactAcceptance(t *testing.T) {
	for _, c := range nativeCompactCases() {
		t.Run(c.name, func(t *testing.T) { t.Parallel(); env := NewTestEnv(t); defer env.Close(); c.run(t, env) })
	}
}
