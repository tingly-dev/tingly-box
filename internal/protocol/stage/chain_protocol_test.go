package stage

import (
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// Anthropic V1 lives only at the edges (upgraded to Beta at the client,
// downgraded at the provider when needed), so every chain entry point rejects
// it before anything executes.
func TestChainRejectsAnthropicV1(t *testing.T) {
	t.Parallel()

	const want = `"anthropic_v1" is handled at the edges`
	beta := &recordingEndpoint{protocol: protocol.TypeAnthropicBeta}
	check := func(t *testing.T, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want containing %q", err, want)
		}
	}

	t.Run("compose terminal", func(t *testing.T) {
		_, err := Compose(&recordingEndpoint{protocol: protocol.TypeAnthropicV1})
		check(t, err)
	})
	t.Run("compose stage", func(t *testing.T) {
		_, err := Compose(beta, &recordingStage{name: "s", protocol: protocol.TypeAnthropicV1})
		check(t, err)
	})
	t.Run("adapt bridge", func(t *testing.T) {
		_, err := Adapt(beta, &testingBridge{source: protocol.TypeAnthropicV1, target: protocol.TypeAnthropicBeta, caps: AllBridgeCapabilities})
		check(t, err)
	})
	t.Run("registry", func(t *testing.T) {
		_, err := NewBridgeRegistry(&testingBridge{source: protocol.TypeAnthropicBeta, target: protocol.TypeAnthropicV1, caps: AllBridgeCapabilities})
		check(t, err)
	})
	t.Run("topology client", func(t *testing.T) {
		registry, err := NewBridgeRegistry()
		if err != nil {
			t.Fatal(err)
		}
		_, err = BuildTopology(TopologyConfig{Terminal: beta, ClientProtocol: protocol.TypeAnthropicV1, Registry: registry})
		check(t, err)
	})
}
