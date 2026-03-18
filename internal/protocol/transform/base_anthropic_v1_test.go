package transform

import (
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestNewAnthropicV1Transform(t *testing.T) {
	avt := NewAnthropicV1Transform()
	assert.Equal(t, "anthropic_v1_adjust", avt.Name())
	assert.NotNil(t, avt)
}

func TestAnthropicV1Transform_Apply_Success(t *testing.T) {
	avt := NewAnthropicV1Transform()
	ctx := &TransformContext{Request: newAnthropicV1Request("claude-3-5-sonnet-20241022", 1024)}

	err := avt.Apply(ctx)
	require.NoError(t, err)
	_, ok := ctx.Request.(*anthropic.MessageNewParams)
	assert.True(t, ok)
}

func TestAnthropicV1Transform_Apply_WrongRequestType(t *testing.T) {
	avt := NewAnthropicV1Transform()
	ctx := &TransformContext{Request: "invalid"}

	err := avt.Apply(ctx)
	require.Error(t, err)

	validationErr, ok := err.(*ValidationError)
	require.True(t, ok)
	assert.Equal(t, "request", validationErr.Field)
	assert.Contains(t, validationErr.Message, "expected anthropic.MessageNewParams")
}

func TestAnthropicV1Transform_Apply_NilRequest(t *testing.T) {
	avt := NewAnthropicV1Transform()
	ctx := &TransformContext{Request: nil}

	err := avt.Apply(ctx)
	require.Error(t, err)
}

func TestAnthropicV1Transform_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		req     *anthropic.MessageNewParams
		wantErr string
		field   string
	}{
		{
			name:    "missing max_tokens",
			req:     &anthropic.MessageNewParams{Model: "model", Messages: []anthropic.MessageParam{}},
			wantErr: "max_tokens is required",
			field:   "max_tokens",
		},
		{
			name:    "empty model",
			req:     &anthropic.MessageNewParams{Model: "", MaxTokens: 1024, Messages: []anthropic.MessageParam{}},
			wantErr: "model is required",
			field:   "model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			avt := NewAnthropicV1Transform()
			ctx := &TransformContext{Request: tt.req}

			err := avt.Apply(ctx)
			require.Error(t, err)

			validationErr, ok := err.(*ValidationError)
			require.True(t, ok)
			assert.Equal(t, tt.field, validationErr.Field)
			assert.Contains(t, validationErr.Message, tt.wantErr)
		})
	}
}

func TestAnthropicV1Transform_normalizeToolSchemas_NoTools(t *testing.T) {
	avt := NewAnthropicV1Transform()
	ctx := &TransformContext{Request: newAnthropicV1Request("claude-3-5-sonnet-20241022", 1024)}

	err := avt.Apply(ctx)
	require.NoError(t, err)
}

func TestAnthropicV1Transform_applyScenarioFlags(t *testing.T) {
	avt := NewAnthropicV1Transform()
	ctx := &TransformContext{
		Request:       newAnthropicV1Request("claude-3-5-sonnet-20241022", 1024),
		ScenarioFlags: &typ.ScenarioFlags{DisableStreamUsage: true},
	}

	err := avt.Apply(ctx)
	require.NoError(t, err)
}

func TestAnthropicV1Transform_normalizeMessages(t *testing.T) {
	avt := NewAnthropicV1Transform()
	req := &anthropic.MessageNewParams{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
			anthropic.NewAssistantMessage(anthropic.NewTextBlock("Hi")),
		},
	}
	ctx := &TransformContext{Request: req}

	err := avt.Apply(ctx)
	require.NoError(t, err)
	assert.Len(t, req.Messages, 2)
}
