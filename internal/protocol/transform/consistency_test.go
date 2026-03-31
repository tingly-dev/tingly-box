package transform

import (
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestNewConsistencyTransform(t *testing.T) {
	ct := NewConsistencyTransform(protocol.TypeOpenAIChat)
	assert.Equal(t, "consistency_normalize", ct.Name())
	assert.Equal(t, protocol.TypeOpenAIChat, ct.targetAPIStyle)
}

func TestConsistencyTransform_Apply_OpenAIChat(t *testing.T) {
	ct := NewConsistencyTransform(protocol.TypeOpenAIChat)

	req := newOpenAIRequest("gpt-4", 1024)
	req.Temperature = param.Opt[float64]{Value: 0.7}
	req.TopP = param.Opt[float64]{Value: 0.9}

	ctx := &TransformContext{Request: req}

	err := ct.Apply(ctx)
	require.NoError(t, err)
	assert.Equal(t, req, ctx.Request)
}

func TestConsistencyTransform_Apply_UnknownAPIStyle(t *testing.T) {
	ct := NewConsistencyTransform("unknown_style")
	ctx := &TransformContext{Request: &openai.ChatCompletionNewParams{}}

	err := ct.Apply(ctx)
	require.NoError(t, err)
}

func TestConsistencyTransform_normalizeToolSchemas(t *testing.T) {
	tests := []struct {
		name      string
		tools     []openai.ChatCompletionToolUnionParam
		wantTools bool
		checkType bool
		wantType  string
	}{
		{
			name:      "no tools",
			tools:     nil,
			wantTools: false,
		},
		{
			name: "tool without type - adds it",
			tools: []openai.ChatCompletionToolUnionParam{
				openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
					Name:        "search",
					Description: param.Opt[string]{Value: "Search"},
					Parameters: map[string]interface{}{
						"properties": map[string]interface{}{
							"query": map[string]interface{}{"type": "string"},
						},
					},
				}),
			},
			wantTools: true,
			checkType: true,
			wantType:  "object",
		},
		{
			name: "empty parameters",
			tools: []openai.ChatCompletionToolUnionParam{
				openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
					Name:        "simple",
					Description: param.Opt[string]{Value: "Simple tool"},
					Parameters:  map[string]interface{}{},
				}),
			},
			wantTools: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct := NewConsistencyTransform(protocol.TypeOpenAIChat)
			req := newOpenAIRequest("gpt-4", 1024)
			req.Tools = tt.tools
			ctx := &TransformContext{Request: req}

			err := ct.Apply(ctx)
			require.NoError(t, err)

			if tt.wantTools {
				assert.NotNil(t, req.Tools)
				if tt.checkType {
					assert.Equal(t, tt.wantType, req.Tools[0].OfFunction.Function.Parameters["type"])
				}
			}
		})
	}
}

func TestConsistencyTransform_normalizeToolSchemas_MultipleTools(t *testing.T) {
	ct := NewConsistencyTransform(protocol.TypeOpenAIChat)

	tool1 := openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
		Name: "tool1",
		Parameters: map[string]interface{}{
			"properties": map[string]interface{}{
				"param1": map[string]interface{}{"type": "string"},
			},
		},
	})

	tool2 := openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
		Name: "tool2",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"param2": map[string]interface{}{"type": "integer"}},
		},
	})

	tool3 := openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
		Name:       "tool3",
		Parameters: map[string]interface{}{},
	})

	req := newOpenAIRequest("gpt-4", 1024)
	req.Tools = []openai.ChatCompletionToolUnionParam{tool1, tool2, tool3}
	ctx := &TransformContext{Request: req}

	err := ct.Apply(ctx)
	require.NoError(t, err)

	// Check that type was added to tools with properties
	assert.Equal(t, "object", req.Tools[0].OfFunction.Function.Parameters["type"])
	assert.Equal(t, "object", req.Tools[1].OfFunction.Function.Parameters["type"])
	// Tool with empty params may have empty struct or nil - just check it exists
	assert.NotNil(t, req.Tools[2].OfFunction.Function.Parameters)
}

func TestConsistencyTransform_validateChat(t *testing.T) {
	tests := []struct {
		name        string
		temperature float64
		maxTokens   int64
		topP        float64
		wantErr     bool
		errField    string
		errContains string
	}{
		{
			name:        "valid request",
			temperature: 0.7,
			maxTokens:   1024,
			topP:        0.9,
			wantErr:     false,
		},
		{
			name:        "temperature too low",
			temperature: -0.5,
			maxTokens:   1024,
			wantErr:     true,
			errField:    "temperature",
			errContains: "between 0 and 2",
		},
		{
			name:        "temperature too high",
			temperature: 2.5,
			maxTokens:   1024,
			wantErr:     true,
			errField:    "temperature",
		},
		{
			name:        "max_tokens negative",
			maxTokens:   -100,
			wantErr:     true,
			errField:    "max_tokens",
			errContains: "non-negative",
		},
		{
			name:        "top_p too low",
			topP:        -0.1,
			maxTokens:   1024,
			wantErr:     true,
			errField:    "top_p",
			errContains: "between 0 and 1",
		},
		{
			name:      "top_p too high",
			topP:      1.5,
			maxTokens: 1024,
			wantErr:   true,
			errField:  "top_p",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct := NewConsistencyTransform(protocol.TypeOpenAIChat)
			req := newOpenAIRequest("gpt-4", tt.maxTokens)
			req.Temperature = param.Opt[float64]{Value: tt.temperature}
			req.TopP = param.Opt[float64]{Value: tt.topP}
			ctx := &TransformContext{Request: req}

			err := ct.Apply(ctx)

			if tt.wantErr {
				require.Error(t, err)
				validationErr, ok := err.(*ValidationError)
				require.True(t, ok)
				assert.Equal(t, tt.errField, validationErr.Field)
				if tt.errContains != "" {
					assert.Contains(t, validationErr.Message, tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConsistencyTransform_applyScenarioFlags_DisableStreamUsage(t *testing.T) {
	ct := NewConsistencyTransform(protocol.TypeOpenAIChat)

	req := newOpenAIRequest("gpt-4", 1024)
	req.StreamOptions = openai.ChatCompletionStreamOptionsParam{
		IncludeUsage: param.Opt[bool]{Value: true},
	}

	ctx := &TransformContext{
		Request:       req,
		ScenarioFlags: &typ.ScenarioFlags{DisableStreamUsage: true},
		IsStreaming:   true,
	}

	err := ct.Apply(ctx)
	require.NoError(t, err)
	assert.False(t, req.StreamOptions.IncludeUsage.Value)
}

func TestConsistencyTransform_applyScenarioFlags_NonStreaming(t *testing.T) {
	ct := NewConsistencyTransform(protocol.TypeOpenAIChat)

	req := newOpenAIRequest("gpt-4", 1024)
	req.StreamOptions = openai.ChatCompletionStreamOptionsParam{
		IncludeUsage: param.Opt[bool]{Value: true},
	}

	ctx := &TransformContext{
		Request:       req,
		ScenarioFlags: &typ.ScenarioFlags{DisableStreamUsage: true},
		IsStreaming:   false,
	}

	err := ct.Apply(ctx)
	require.NoError(t, err)
	assert.True(t, req.StreamOptions.IncludeUsage.Value) // Should not modify
}

func TestConsistencyTransform_applyScenarioFlags_StoresInExtraFields(t *testing.T) {
	ct := NewConsistencyTransform(protocol.TypeOpenAIChat)
	flags := &typ.ScenarioFlags{DisableStreamUsage: true}

	req := newOpenAIRequest("gpt-4", 1024)
	req.StreamOptions = openai.ChatCompletionStreamOptionsParam{
		IncludeUsage: param.Opt[bool]{Value: true},
	}

	ctx := &TransformContext{
		Request:       req,
		ScenarioFlags: flags,
		IsStreaming:   true,
	}

	err := ct.Apply(ctx)
	require.NoError(t, err)

	extraFields := req.ExtraFields()
	require.NotNil(t, extraFields)
	storedFlags, ok := extraFields["scenario_flags"].(*typ.ScenarioFlags)
	require.True(t, ok)
	assert.Equal(t, flags, storedFlags)
}

func TestConsistencyTransform_normalizeMessages_NoMessages(t *testing.T) {
	ct := NewConsistencyTransform(protocol.TypeOpenAIChat)
	req := newOpenAIRequest("gpt-4", 1024)
	req.Messages = []openai.ChatCompletionMessageParamUnion{}
	ctx := &TransformContext{Request: req}

	err := ct.Apply(ctx)
	require.NoError(t, err)
}

func TestConsistencyTransform_WrongRequestType(t *testing.T) {
	ct := NewConsistencyTransform(protocol.TypeOpenAIChat)
	ctx := &TransformContext{Request: "not a chat completion"}

	err := ct.Apply(ctx)
	// Should return a ValidationError for wrong request type
	require.Error(t, err)
	validationErr, ok := err.(*ValidationError)
	require.True(t, ok, "error should be a ValidationError")
	assert.Equal(t, "request", validationErr.Field)
}

func TestConsistencyTransform_Placeholders(t *testing.T) {
	tests := []struct {
		name  string
		style protocol.APIType
		req   interface{}
	}{
		{
			"Responses API",
			protocol.TypeOpenAIResponses,
			&responses.ResponseNewParams{
				Model: "gpt-4o",
			},
		},
		{
			"Anthropic V1",
			protocol.TypeAnthropicV1,
			&anthropic.MessageNewParams{
				Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
				MaxTokens: int64(1024),
				Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("test"))},
			},
		},
		{
			"Anthropic Beta",
			protocol.TypeAnthropicBeta,
			&anthropic.BetaMessageNewParams{
				Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
				MaxTokens: int64(1024),
				Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("test"))},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct := NewConsistencyTransform(tt.style)
			ctx := &TransformContext{Request: tt.req}

			err := ct.Apply(ctx)
			require.NoError(t, err)
		})
	}
}

func TestValidationError_Error(t *testing.T) {
	tests := []struct {
		name         string
		err          *ValidationError
		wantContains []string
		notContains  []string
	}{
		{
			name: "with value",
			err: &ValidationError{
				Field:   "temperature",
				Message: "must be between 0 and 2",
				Value:   2.5,
			},
			wantContains: []string{"validation error", "temperature", "2.5"},
		},
		{
			name: "without value",
			err: &ValidationError{
				Field:   "model",
				Message: "is required",
			},
			wantContains: []string{"validation error", "model"},
			notContains:  []string{"value:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errStr := tt.err.Error()
			for _, s := range tt.wantContains {
				assert.Contains(t, errStr, s)
			}
			for _, s := range tt.notContains {
				assert.NotContains(t, errStr, s)
			}
		})
	}
}
