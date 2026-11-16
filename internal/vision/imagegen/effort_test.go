package imagegen

import (
	"bytes"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestImageReasoningEffortCarry(t *testing.T) {
	for _, effort := range []string{"", "auto", "low", "medium", "high"} {
		t.Run(effort, func(t *testing.T) {
			want := effort
			if want == "" {
				want = "auto"
			}
			generation := openai.ImageGenerateParams{Prompt: "a fox"}
			generation.SetExtraFields(map[string]any{"other": "preserved"})
			SetReasoningEffort(&generation, effort)
			normalized := RequestFromOpenAI(&generation)
			assert.Equal(t, want, normalized.ReasoningEffort)
			roundTrip := normalized.ToOpenAIParams()
			assert.Equal(t, want, ReasoningEffort(&roundTrip))
			assert.Equal(t, "preserved", generation.ExtraFields()["other"])

			edit := openai.ImageEditParams{Prompt: "a fox"}
			edit.Image.OfFile = bytes.NewReader([]byte("image"))
			SetReasoningEffort(&edit, effort)
			normalizedEdit, err := EditRequestFromOpenAI(&edit)
			require.NoError(t, err)
			assert.Equal(t, want, normalizedEdit.ReasoningEffort)
		})
	}
}

func TestOpenAIImageReasoningEffortMapping(t *testing.T) {
	for _, tc := range []struct {
		name, base, model, effort, want string
	}{
		{"auto keeps upstream default", "https://api.openai.com/v1", "gpt-image-2", "auto", ""},
		{"low", "https://api.openai.com/v1", "gpt-image-2", "low", "low"},
		{"medium", "https://api.openai.com/v1", "gpt-image-2", "medium", "medium"},
		{"high", "https://api.openai.com/v1", "gpt-image-2", "high", "high"},
		{"unknown effort ignored", "https://api.openai.com/v1", "gpt-image-2", "future", ""},
		{"unsupported model", "https://api.openai.com/v1", "dall-e-3", "high", ""},
		{"compat ignored", "https://compatible.example/v1", "gpt-image-2", "high", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			extra := map[string]any{ReasoningEffortField: tc.effort, "other": "preserved"}
			out := OpenAIImageExtras(&typ.Provider{APIBase: tc.base}, tc.model, extra)
			assert.NotContains(t, out, ReasoningEffortField)
			assert.Equal(t, "preserved", out["other"])
			assert.Equal(t, tc.effort, extra[ReasoningEffortField], "mapping must not mutate the original request")
			if tc.want == "" {
				assert.NotContains(t, out, "complexity")
			} else {
				assert.Equal(t, tc.want, out["complexity"])
			}
		})
	}
}
