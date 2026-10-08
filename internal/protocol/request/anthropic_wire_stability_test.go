package request

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"
)

// A client's tool definitions carry JSON Schema keys the SDK does not model
// ($schema, additionalProperties, ...). They are kept as extra fields and
// must re-encode byte for byte on every request: tools sit at the head of
// the cacheable prefix, and a provider whose prompt cache is keyed on the
// request bytes misses the whole conversation when they permute. The
// tingly-dev/anthropic-sdk-go fork emits extra fields in sorted key order
// for this; this test pins that the request we send is stable.
const wireStabilityBody = `{"model":"m","max_tokens":64,
"tools":[
 {"name":"Bash","description":"run","input_schema":{"type":"object","properties":{"command":{"type":"string"}},"required":["command"],"additionalProperties":false,"$schema":"http://json-schema.org/draft-07/schema#","title":"Bash"}},
 {"name":"Read","description":"read","input_schema":{"type":"object","properties":{"file_path":{"type":"string"}},"additionalProperties":false,"$schema":"http://json-schema.org/draft-07/schema#"},"cache_control":{"type":"ephemeral"}}],
"messages":[{"role":"user","content":"hi"}]}`

func TestAnthropicRequestWireIsStableAcrossMarshals(t *testing.T) {
	t.Run("v1", func(t *testing.T) {
		var v1 anthropic.MessageNewParams
		require.NoError(t, json.Unmarshal([]byte(wireStabilityBody), &v1))
		first, err := json.Marshal(&v1)
		require.NoError(t, err)
		require.Contains(t, string(first), `"additionalProperties":false`, "extra schema keys must reach the wire")
		for range 100 {
			again, err := json.Marshal(&v1)
			require.NoError(t, err)
			require.Equal(t, string(first), string(again), "the same request must encode to the same bytes")
		}
	})
	t.Run("beta", func(t *testing.T) {
		var beta anthropic.BetaMessageNewParams
		require.NoError(t, json.Unmarshal([]byte(wireStabilityBody), &beta))
		first, err := json.Marshal(&beta)
		require.NoError(t, err)
		require.Contains(t, string(first), `"additionalProperties":false`, "extra schema keys must reach the wire")
		for range 100 {
			again, err := json.Marshal(&beta)
			require.NoError(t, err)
			require.Equal(t, string(first), string(again), "the same request must encode to the same bytes")
		}
	})
	t.Run("v1_beta_v1", func(t *testing.T) {
		var v1 anthropic.MessageNewParams
		require.NoError(t, json.Unmarshal([]byte(wireStabilityBody), &v1))
		direct, err := json.Marshal(&v1)
		require.NoError(t, err)
		beta, err := ConvertAnthropicV1ToBetaRequestWithError(&v1)
		require.NoError(t, err)
		back, err := ConvertAnthropicBetaToV1Request(beta)
		require.NoError(t, err)
		viaBeta, err := json.Marshal(back)
		require.NoError(t, err)
		require.Equal(t, string(direct), string(viaBeta), "the V1 wire must not change through the Beta edge")
	})
}
