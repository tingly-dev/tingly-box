package protocoltest

import "testing"

// The semantic matrix (every pair x scenario x streaming mode) and its
// idempotence section, run under go test so refactors are checked by the same
// loop as unit tests rather than only by the harness CLI.

func TestMatrixSingle(t *testing.T) {
	t.Parallel()
	DefaultMatrix().Run(t)
}

func TestMatrixIdempotent(t *testing.T) {
	t.Parallel()
	DefaultMatrix().RunIdempotent(t)
}
