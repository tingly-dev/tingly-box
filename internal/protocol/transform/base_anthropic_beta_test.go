package transform

import (
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestNewAnthropicBetaTransform(t *testing.T) {
	abt := NewAnthropicBetaTransform()
	assert.Equal(t, "anthropic_beta_adjust", abt.Name())
	assert.NotNil(t, abt)
}

func TestAnthropicBetaTransform_Apply_Success(t *testing.T) {
	abt := NewAnthropicBetaTransform()
	ctx := &TransformContext{Request: newAnthropicBetaRequest("claude-3-5-sonnet-20241022", 1024)}

	err := abt.Apply(ctx)
	require.NoError(t, err)
	_, ok := ctx.Request.(*anthropic.BetaMessageNewParams)
	assert.True(t, ok)
}

func TestAnthropicBetaTransform_Apply_WrongRequestType(t *testing.T) {
	abt := NewAnthropicBetaTransform()
	ctx := &TransformContext{Request: "invalid"}

	err := abt.Apply(ctx)
	require.Error(t, err)

	validationErr, ok := err.(*ValidationError)
	require.True(t, ok)
	assert.Equal(t, "request", validationErr.Field)
}

func TestAnthropicBetaTransform_Apply_NilRequest(t *testing.T) {
	abt := NewAnthropicBetaTransform()
	ctx := &TransformContext{Request: nil}

	err := abt.Apply(ctx)
	require.Error(t, err)
}

func TestAnthropicBetaTransform_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		req     *anthropic.BetaMessageNewParams
		wantErr string
		field   string
	}{
		{
			name:    "missing max_tokens",
			req:     &anthropic.BetaMessageNewParams{Model: "model", Messages: []anthropic.BetaMessageParam{}},
			wantErr: "max_tokens is required",
			field:   "max_tokens",
		},
		{
			name:    "empty model",
			req:     &anthropic.BetaMessageNewParams{Model: "", MaxTokens: 1024, Messages: []anthropic.BetaMessageParam{}},
			wantErr: "model is required",
			field:   "model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			abt := NewAnthropicBetaTransform()
			ctx := &TransformContext{Request: tt.req}

			err := abt.Apply(ctx)
			require.Error(t, err)

			validationErr, ok := err.(*ValidationError)
			require.True(t, ok)
			assert.Equal(t, tt.field, validationErr.Field)
			assert.Contains(t, validationErr.Message, tt.wantErr)
		})
	}
}

func TestAnthropicBetaTransform_normalizeToolSchemas_NoTools(t *testing.T) {
	abt := NewAnthropicBetaTransform()
	ctx := &TransformContext{Request: newAnthropicBetaRequest("claude-3-5-sonnet-20241022", 1024)}

	err := abt.Apply(ctx)
	require.NoError(t, err)
}

func TestAnthropicBetaTransform_normalizeToolSchemas_WithTools(t *testing.T) {
	abt := NewAnthropicBetaTransform()

	tool := anthropic.BetaToolParam{
		Name: "search",
		InputSchema: anthropic.BetaToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query"},
			},
		},
	}

	req := newAnthropicBetaRequest("claude-3-5-sonnet-20241022", 1024)
	req.Tools = []anthropic.BetaToolUnionParam{{OfTool: &tool}}
	ctx := &TransformContext{Request: req}

	err := abt.Apply(ctx)
	require.NoError(t, err)
	assert.Len(t, req.Tools, 1)
}

func TestAnthropicBetaTransform_applyScenarioFlags(t *testing.T) {
	abt := NewAnthropicBetaTransform()
	ctx := &TransformContext{
		Request:       newAnthropicBetaRequest("claude-3-5-sonnet-20241022", 1024),
		ScenarioFlags: &typ.ScenarioFlags{DisableStreamUsage: true},
	}

	err := abt.Apply(ctx)
	require.NoError(t, err)
}

func TestAnthropicBetaTransform_WithStopSequences(t *testing.T) {
	abt := NewAnthropicBetaTransform()

	req := newAnthropicBetaRequest("claude-3-5-sonnet-20241022", 1024)
	req.StopSequences = []string{"\n\n", "STOP"}
	ctx := &TransformContext{Request: req}

	err := abt.Apply(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, len(req.StopSequences))
}

func TestAnthropicBetaTransform_MultipleMessages(t *testing.T) {
	abt := NewAnthropicBetaTransform()

	req := &anthropic.BetaMessageNewParams{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		Messages: []anthropic.BetaMessageParam{
			{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("First")}},
			{Role: anthropic.BetaMessageParamRoleAssistant, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("Response")}},
			{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("Second")}},
		},
	}
	ctx := &TransformContext{Request: req}

	err := abt.Apply(ctx)
	require.NoError(t, err)
	assert.Len(t, req.Messages, 3)
}
