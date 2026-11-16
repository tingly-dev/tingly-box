package imagegen

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageReasoningEffortOmission(t *testing.T) {
	generation := openai.ImageGenerateParams{Prompt: "a fox"}
	generation.SetExtraFields(map[string]any{"other": "preserved"})
	SetReasoningEffort(&generation, "high")
	// The serving adapter receives a value copy, as it does in production.
	outbound := generation
	OmitReasoningEffort(&outbound)
	body, err := json.Marshal(outbound)
	require.NoError(t, err)
	assert.NotContains(t, string(body), ReasoningEffortField)
	assert.Equal(t, "auto", ReasoningEffort(&outbound))
	assert.Equal(t, "preserved", outbound.ExtraFields()["other"])
	assert.Equal(t, "high", ReasoningEffort(&generation), "omitting upstream must not mutate the original request")

	// The edit SDK uses the same extras mechanism before multipart encoding.
	edit := openai.ImageEditParams{}
	SetReasoningEffort(&edit, "high")
	OmitReasoningEffort(&edit)
	assert.NotContains(t, edit.ExtraFields(), ReasoningEffortField)
}
