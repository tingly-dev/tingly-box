package protocoltest

import "testing"

// TestVendorTransforms drives cached and no-cache requests to virtual
// providers whose APIBase matches real vendor discriminators (api.openai.com,
// api.deepseek.com, ...), verifying the explicit-prompt-cache allowlist on
// both OpenAI wire shapes end-to-end rather than against a hand-built request
// struct. The case implementation is shared with `harness matrix --mode=vendor`.
func TestVendorTransforms(t *testing.T) {
	m := DefaultMatrix()
	for _, fx := range vendorFixtures {
		for _, target := range vendorTargets {
			for _, streaming := range m.Streaming {
				t.Run(vendorCaseName(fx, target, streaming), func(t *testing.T) {
					t.Parallel()
					env := NewTestEnv(t)
					defer env.Close()
					runVendorTransformCase(t, env, fx, target, streaming)
				})
			}
		}
	}
}
