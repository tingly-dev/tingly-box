package protocoltest

import "testing"

func TestResponsesSnapshotRetention(t *testing.T) {
	for _, c := range responsesSnapshotCases() {
		t.Run(c.name, func(t *testing.T) { t.Parallel(); env := NewTestEnv(t); defer env.Close(); c.run(t, env) })
	}
}
