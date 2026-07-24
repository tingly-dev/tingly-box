package protocoltest

import "testing"

// The request-content-shape regression suite itself lives in content_shapes.go
// (shared with the CLI `harness matrix --mode=content_shapes`). This file is
// just the go-test entry point.

// TestContentShapes drives every request content-shape regression case
// through the real gateway and asserts on the request actually forwarded
// upstream. *testing.T satisfies the flagTB the cases use.
func TestContentShapes(t *testing.T) {
	for _, c := range contentShapeCases() {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			env := NewTestEnv(t)
			defer env.Close()
			c.run(t, env)
		})
	}
}
