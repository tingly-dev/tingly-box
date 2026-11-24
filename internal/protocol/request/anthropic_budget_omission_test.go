package request

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/stretchr/testify/require"
)

func TestAnthropicConversionPreservesExplicitBudgetOmission(t *testing.T) {
	for _, omitted := range []bool{false, true} {
		t.Run(map[bool]string{false: "preserve_zero", true: "omit"}[omitted], func(t *testing.T) {
			v1 := &anthropic.MessageNewParams{Thinking: anthropic.ThinkingConfigParamOfEnabled(0)}
			if omitted {
				v1.Thinking.OfEnabled.SetExtraFields(map[string]any{"budget_tokens": param.Omit})
			}
			beta, err := ConvertAnthropicV1ToBetaRequestWithError(v1)
			require.NoError(t, err)
			back, err := ConvertAnthropicBetaToV1Request(beta)
			require.NoError(t, err)
			for _, req := range []any{v1, beta, back} {
				data, err := json.Marshal(req)
				require.NoError(t, err)
				var body map[string]any
				require.NoError(t, json.Unmarshal(data, &body))
				thinking := body["thinking"].(map[string]any)
				require.Equal(t, "enabled", thinking["type"])
				value, present := thinking["budget_tokens"]
				if omitted {
					require.False(t, present)
				} else {
					require.True(t, present)
					require.Equal(t, float64(0), value)
				}
			}
		})
	}
}
