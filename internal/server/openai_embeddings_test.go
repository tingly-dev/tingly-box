package server

import (
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
)

func TestIsEmbeddingInputEmpty(t *testing.T) {
	cases := []struct {
		name  string
		input openai.EmbeddingNewParamsInputUnion
		want  bool
	}{
		{
			name:  "empty union",
			input: openai.EmbeddingNewParamsInputUnion{},
			want:  true,
		},
		{
			name:  "empty string",
			input: openai.EmbeddingNewParamsInputUnion{OfString: param.NewOpt("")},
			want:  true,
		},
		{
			name:  "non-empty string",
			input: openai.EmbeddingNewParamsInputUnion{OfString: param.NewOpt("hello")},
			want:  false,
		},
		{
			name:  "non-empty string array",
			input: openai.EmbeddingNewParamsInputUnion{OfArrayOfStrings: []string{"a", "b"}},
			want:  false,
		},
		{
			name:  "non-empty token array",
			input: openai.EmbeddingNewParamsInputUnion{OfArrayOfTokens: []int64{1, 2, 3}},
			want:  false,
		},
		{
			name:  "non-empty token array of arrays",
			input: openai.EmbeddingNewParamsInputUnion{OfArrayOfTokenArrays: [][]int64{{1}, {2}}},
			want:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isEmbeddingInputEmpty(tc.input); got != tc.want {
				t.Fatalf("isEmbeddingInputEmpty(%+v) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
