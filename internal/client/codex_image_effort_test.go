package client

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tingly-dev/tingly-box/internal/vision/imagegen"
)

func TestCodexImageReasoningEffort(t *testing.T) {
	for _, tc := range []struct{ effort, want string }{
		{"", "low"}, {"auto", "low"}, {"low", "low"}, {"medium", "medium"}, {"high", "high"}, {"future", ""},
	} {
		t.Run(tc.effort, func(t *testing.T) {
			generation := openai.ImageGenerateParams{Model: "gpt-5", Prompt: "a fox"}
			imagegen.SetReasoningEffort(&generation, tc.effort)
			generationParams := (&CodexClient{}).buildImageGenerationResponsesRequest(generation)
			edit := openai.ImageEditParams{Model: "gpt-image-2", Prompt: "a fox"}
			edit.Image.OfFile = bytes.NewReader(testPNGBytes)
			imagegen.SetReasoningEffort(&edit, tc.effort)
			editParams, err := buildImageEditResponsesRequest(&edit)
			require.NoError(t, err)
			for _, params := range []any{generationParams, editParams} {
				body, err := json.Marshal(params)
				require.NoError(t, err)
				wire, err := (&codexRoundTripper{}).filterField(body)
				require.NoError(t, err)
				assert.Equal(t, tc.want, gjson.GetBytes(wire, "reasoning.effort").String())
				assert.False(t, gjson.GetBytes(wire, "reasoning_effort").Exists())
				assert.Equal(t, "image_generation", gjson.GetBytes(wire, "tools.0.type").String())
			}

			// Native edits ignore the hint and keep their existing route.
			edit.Image.OfFile = bytes.NewReader(testPNGBytes)
			native, err := buildCodexImageEditRequest(&edit)
			require.NoError(t, err)
			body, err := json.Marshal(native)
			require.NoError(t, err)
			assert.NotContains(t, string(body), "reasoning")
		})
	}
}
