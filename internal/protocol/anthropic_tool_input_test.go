package protocol

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnthropicToolInputJSONPreservesWireAndConstructedValues(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		input     any
		want      string
	}{
		{"stream union rounded by SDK", `{"input":{"id":9007199254740993}}`, map[string]any{"id": float64(9007199254740993)}, `{"id":9007199254740993}`},
		{"constructed number", "", map[string]any{"id": json.Number("9007199254740993")}, `{"id":9007199254740993}`},
		{"constructed raw input", "", json.RawMessage(`{"id":9007199254740993}`), `{"id":9007199254740993}`},
		{"missing input", "", nil, `{}`},
		{"null stream input", `{"input":null}`, nil, `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) { require.Equal(t, tc.want, AnthropicToolInputJSON(tc.raw, tc.input)) })
	}
}
