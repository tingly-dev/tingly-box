package protocoltest

import "testing"

// TestCachePrefix drives the cross-request prompt-cache checks through the real
// gateway for every client shape × target protocol, plus the Codex provider
// boundary. The case implementation is shared with
// `harness matrix --mode=cache_prefix`.
func TestCachePrefix(t *testing.T) {
	m := DefaultMatrix()
	for _, c := range cachePrefixClients() {
		for _, target := range m.cachePrefixTargets() {
			for _, streaming := range m.Streaming {
				t.Run("generic/"+c.name+"/"+string(target)+"/"+streamMode(streaming), func(t *testing.T) {
					t.Parallel()
					env := NewTestEnv(t)
					defer env.Close()
					runCachePrefixCase(t, env, c, target, streaming)
				})
			}
		}
		for _, streaming := range m.Streaming {
			t.Run("codex/"+c.name+"/"+streamMode(streaming), func(t *testing.T) {
				t.Parallel()
				env := NewTestEnv(t)
				defer env.Close()
				runCodexBoundaryCachePrefixCase(t, env, c, streaming)
			})
		}
	}
}
