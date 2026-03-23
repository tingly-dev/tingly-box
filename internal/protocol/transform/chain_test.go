package transform

import (
	"errors"
	"fmt"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	shared "github.com/openai/openai-go/v3/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Mock transform for testing
type mockTransform struct {
	name       string
	shouldFail bool
	errorMsg   string
	applyCount int
}

func (m *mockTransform) Name() string {
	return m.name
}

func (m *mockTransform) Apply(ctx *TransformContext) error {
	m.applyCount++
	if m.shouldFail {
		return errors.New(m.errorMsg)
	}
	return nil
}

// Custom transforms for specific test cases
type customValidationErrorTransform struct{ name string }

func (t *customValidationErrorTransform) Name() string { return t.name }

func (t *customValidationErrorTransform) Apply(ctx *TransformContext) error {
	return &ValidationError{Field: "temperature", Message: "must be between 0 and 2", Value: 3.5}
}

type requestModifyTransform struct{ name string }

func (t *requestModifyTransform) Name() string { return t.name }

func (t *requestModifyTransform) Apply(ctx *TransformContext) error {
	req, ok := ctx.Request.(*openai.ChatCompletionNewParams)
	if !ok {
		return fmt.Errorf("unexpected request type")
	}
	req.MaxTokens = param.Opt[int64]{Value: 2048}
	req.Temperature = param.Opt[float64]{Value: 0.9}
	return nil
}

// Basic chain tests
func TestNewTransformChain(t *testing.T) {
	chain := NewTransformChain([]Transform{
		&mockTransform{name: "transform1"},
		&mockTransform{name: "transform2"},
	})
	assert.NotNil(t, chain)
	assert.Equal(t, 2, chain.Length())
}

func TestTransformChain_Execute_Success(t *testing.T) {
	transforms := []*mockTransform{
		{name: "first"},
		{name: "second"},
		{name: "third"},
	}

	chain := NewTransformChain([]Transform{transforms[0], transforms[1], transforms[2]})
	ctx := &TransformContext{Request: newOpenAIRequest("gpt-4", 1024)}

	result, err := chain.Execute(ctx)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []string{"first", "second", "third"}, result.TransformSteps)

	for _, tf := range transforms {
		assert.Equal(t, 1, tf.applyCount)
	}
}

func TestTransformChain_Execute_InitializesFields(t *testing.T) {
	chain := NewTransformChain([]Transform{&mockTransform{name: "test"}})
	ctx := &TransformContext{Request: &openai.ChatCompletionNewParams{}}

	result, err := chain.Execute(ctx)

	require.NoError(t, err)
	assert.NotNil(t, result.TransformSteps)
	assert.NotNil(t, result.Extra)
	assert.NotNil(t, result.OriginalRequest)
	assert.Equal(t, ctx.Request, result.OriginalRequest)
}

func TestTransformChain_Execute_PreservesExistingFields(t *testing.T) {
	chain := NewTransformChain([]Transform{&mockTransform{name: "test"}})

	originalReq := &openai.ChatCompletionNewParams{}
	existingSteps := []string{"existing_step"}
	existingExtra := map[string]interface{}{"key": "value"}

	ctx := &TransformContext{
		Request:         &openai.ChatCompletionNewParams{},
		OriginalRequest: originalReq,
		TransformSteps:  existingSteps,
		Extra:           existingExtra,
	}

	result, err := chain.Execute(ctx)

	require.NoError(t, err)
	assert.Equal(t, originalReq, result.OriginalRequest)
	assert.Equal(t, []string{"existing_step", "test"}, result.TransformSteps)
	assert.Equal(t, "value", result.Extra["key"])
}

func TestTransformChain_Execute_TransformFails(t *testing.T) {
	transforms := []*mockTransform{
		{name: "first"},
		{name: "failing", shouldFail: true, errorMsg: "transform failed"},
		{name: "third"},
	}

	chain := NewTransformChain([]Transform{transforms[0], transforms[1], transforms[2]})
	ctx := &TransformContext{Request: &openai.ChatCompletionNewParams{}}

	result, err := chain.Execute(ctx)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "'failing' failed")

	assert.Equal(t, 1, transforms[0].applyCount)
	assert.Equal(t, 1, transforms[1].applyCount)
	assert.Equal(t, 0, transforms[2].applyCount)
}

func TestTransformChain_Execute_EmptyChain(t *testing.T) {
	chain := NewTransformChain([]Transform{})
	ctx := &TransformContext{Request: &openai.ChatCompletionNewParams{}}

	result, err := chain.Execute(ctx)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotNil(t, result.TransformSteps)
	assert.NotNil(t, result.Extra)
}

// Context field preservation tests (table-driven)
func TestTransformChain_ContextPreservation(t *testing.T) {
	tests := []struct {
		name           string
		setupCtx       func() *TransformContext
		verifyCtx      func(*testing.T, *TransformContext)
		wantEmptySteps bool
	}{
		{
			name: "ProviderURL preserved",
			setupCtx: func() *TransformContext {
				return &TransformContext{
					Request:     &openai.ChatCompletionNewParams{},
					ProviderURL: "api.deepseek.com",
				}
			},
			verifyCtx: func(t *testing.T, result *TransformContext) {
				assert.Equal(t, "api.deepseek.com", result.ProviderURL)
			},
		},
		{
			name: "IsStreaming preserved",
			setupCtx: func() *TransformContext {
				return &TransformContext{
					Request:     &openai.ChatCompletionNewParams{},
					IsStreaming: true,
				}
			},
			verifyCtx: func(t *testing.T, result *TransformContext) {
				assert.True(t, result.IsStreaming)
			},
		},
		{
			name: "ScenarioFlags preserved",
			setupCtx: func() *TransformContext {
				return &TransformContext{
					Request:       &openai.ChatCompletionNewParams{},
					ScenarioFlags: &typ.ScenarioFlags{DisableStreamUsage: true},
				}
			},
			verifyCtx: func(t *testing.T, result *TransformContext) {
				require.NotNil(t, result.ScenarioFlags)
				assert.True(t, result.ScenarioFlags.DisableStreamUsage)
			},
		},
		{
			name: "Extra map preserved",
			setupCtx: func() *TransformContext {
				return &TransformContext{
					Request: &openai.ChatCompletionNewParams{},
					Extra: map[string]interface{}{
						"key1": "value1",
						"key2": "value2",
					},
				}
			},
			verifyCtx: func(t *testing.T, result *TransformContext) {
				assert.Equal(t, "value1", result.Extra["key1"])
				assert.Equal(t, "value2", result.Extra["key2"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := NewTransformChain([]Transform{&mockTransform{name: "test"}})
			ctx := tt.setupCtx()

			result, err := chain.Execute(ctx)
			require.NoError(t, err)
			tt.verifyCtx(t, result)
		})
	}
}

// Chain manipulation tests
func TestTransformChain_Add(t *testing.T) {
	transform1 := &mockTransform{name: "first"}
	transform2 := &mockTransform{name: "second"}

	chain := NewTransformChain([]Transform{transform1})
	assert.Equal(t, 1, chain.Length())

	chain.Add(transform2)
	assert.Equal(t, 2, chain.Length())
}

func TestTransformChain_GetTransforms(t *testing.T) {
	transform1 := &mockTransform{name: "first"}
	transform2 := &mockTransform{name: "second"}

	chain := NewTransformChain([]Transform{transform1, transform2})
	transforms := chain.GetTransforms()

	assert.Equal(t, 2, len(transforms))
	assert.Equal(t, "first", transforms[0].Name())
	assert.Equal(t, "second", transforms[1].Name())
}

func TestTransformChain_GetTransforms_ReturnsCopy(t *testing.T) {
	transform1 := &mockTransform{name: "first"}
	chain := NewTransformChain([]Transform{transform1})

	transforms := chain.GetTransforms()
	transforms[0] = &mockTransform{name: "modified"}

	original := chain.GetTransforms()
	assert.Equal(t, "first", original[0].Name())
}

func TestTransformChain_Length(t *testing.T) {
	chain := NewTransformChain([]Transform{})
	assert.Equal(t, 0, chain.Length())

	for i := 1; i <= 5; i++ {
		chain.Add(&mockTransform{name: "transform"})
		assert.Equal(t, i, chain.Length())
	}
}

// Integration tests with real transforms
func TestTransformChain_Integration_RealTransforms(t *testing.T) {
	baseTransform := NewBaseTransform(TargetAPIStyleOpenAIChat)
	vendorTransform := NewVendorTransform("api.openai.com")

	chain := NewTransformChain([]Transform{baseTransform, vendorTransform})

	ctx := &TransformContext{
		Request:        newOpenAIRequest("gpt-4", 1024),
		ProviderURL:    "api.openai.com",
		ScenarioFlags:  &typ.ScenarioFlags{},
		IsStreaming:    false,
		TransformSteps: []string{},
		Extra:          make(map[string]interface{}),
	}

	result, err := chain.Execute(ctx)

	require.NoError(t, err)
	assert.Equal(t, []string{"base_convert", "vendor_adjust"}, result.TransformSteps)

	_, ok := result.Request.(*openai.ChatCompletionNewParams)
	assert.True(t, ok)
}

func TestTransformChain_Integration_WithScenarioFlags(t *testing.T) {
	flags := &typ.ScenarioFlags{DisableStreamUsage: true}

	baseTransform := NewBaseTransform(TargetAPIStyleOpenAIChat)
	consistencyTransform := NewConsistencyTransform(TargetAPIStyleOpenAIChat)

	chain := NewTransformChain([]Transform{baseTransform, consistencyTransform})

	req := newOpenAIRequest("gpt-4", 1024)
	req.StreamOptions = openai.ChatCompletionStreamOptionsParam{
		IncludeUsage: param.Opt[bool]{Value: true},
	}

	ctx := &TransformContext{
		Request:        req,
		IsStreaming:    true,
		ScenarioFlags:  flags,
		TransformSteps: []string{},
		Extra:          make(map[string]interface{}),
	}

	result, err := chain.Execute(ctx)

	require.NoError(t, err)
	assert.True(t, result.ScenarioFlags.DisableStreamUsage)
	assert.False(t, req.StreamOptions.IncludeUsage.Value)
}

func TestTransformChain_Integration_FullChain(t *testing.T) {
	baseTransform := NewBaseTransform(TargetAPIStyleOpenAIChat)
	consistencyTransform := NewConsistencyTransform(TargetAPIStyleOpenAIChat)
	vendorTransform := NewVendorTransform("api.openai.com")

	chain := NewTransformChain([]Transform{
		baseTransform,
		consistencyTransform,
		vendorTransform,
	})

	req := newOpenAIRequest("gpt-4", 1024)
	req.Temperature = param.Opt[float64]{Value: 0.7}

	ctx := &TransformContext{
		Request:        req,
		ProviderURL:    "api.openai.com",
		IsStreaming:    false,
		ScenarioFlags:  &typ.ScenarioFlags{},
		TransformSteps: []string{},
		Extra:          make(map[string]interface{}),
	}

	result, err := chain.Execute(ctx)

	require.NoError(t, err)
	assert.Equal(t, []string{"base_convert", "consistency_normalize", "vendor_adjust"}, result.TransformSteps)

	config, ok := result.Extra["openaiConfig"].(*protocol.OpenAIConfig)
	assert.True(t, ok)
	assert.NotNil(t, config)
}

func TestTransformChain_Integration_WithTools(t *testing.T) {
	baseTransform := NewBaseTransform(TargetAPIStyleOpenAIChat)
	consistencyTransform := NewConsistencyTransform(TargetAPIStyleOpenAIChat)
	vendorTransform := NewVendorTransform("api.openai.com")

	chain := NewTransformChain([]Transform{baseTransform, consistencyTransform, vendorTransform})

	functionDef := shared.FunctionDefinitionParam{
		Name:        "test_tool",
		Description: param.Opt[string]{Value: "A test tool"},
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"param1": map[string]interface{}{"type": "string", "description": "A parameter"},
			},
		},
	}

	req := newOpenAIRequest("gpt-4", 1024)
	req.Tools = []openai.ChatCompletionToolUnionParam{openai.ChatCompletionFunctionTool(functionDef)}

	ctx := &TransformContext{
		Request:        req,
		ProviderURL:    "api.openai.com",
		IsStreaming:    false,
		ScenarioFlags:  &typ.ScenarioFlags{},
		TransformSteps: []string{},
		Extra:          make(map[string]interface{}),
	}

	result, err := chain.Execute(ctx)

	require.NoError(t, err)
	transformedReq, ok := result.Request.(*openai.ChatCompletionNewParams)
	assert.True(t, ok)
	assert.Equal(t, 1, len(transformedReq.Tools))
}

func TestTransformChain_Integration_ResponsesAPI(t *testing.T) {
	baseTransform := NewBaseTransform(TargetAPIStyleOpenAIResponses)
	vendorTransform := NewVendorTransform("api.openai.com")

	chain := NewTransformChain([]Transform{baseTransform, vendorTransform})

	responsesReq := responses.ResponseNewParams{
		Model:           shared.ResponsesModel("gpt-4o"),
		Input:           responses.ResponseNewParamsInputUnion{OfString: param.Opt[string]{Value: "Hello"}},
		MaxOutputTokens: param.Opt[int64]{Value: 1024},
	}

	ctx := &TransformContext{
		Request:        &responsesReq,
		ProviderURL:    "api.openai.com",
		IsStreaming:    false,
		ScenarioFlags:  &typ.ScenarioFlags{},
		TransformSteps: []string{},
		Extra:          make(map[string]interface{}),
	}

	result, err := chain.Execute(ctx)

	require.NoError(t, err)
	assert.Equal(t, []string{"base_convert", "vendor_adjust"}, result.TransformSteps)
	_, ok := result.Request.(*responses.ResponseNewParams)
	assert.True(t, ok)
}

// Edge cases
func TestTransformChain_EdgeCase_LargeChain(t *testing.T) {
	var transforms []Transform
	for i := 0; i < 10; i++ {
		transforms = append(transforms, &mockTransform{name: fmt.Sprintf("transform%d", i)})
	}

	chain := NewTransformChain(transforms)
	ctx := &TransformContext{Request: &openai.ChatCompletionNewParams{}}

	result, err := chain.Execute(ctx)

	require.NoError(t, err)
	assert.Equal(t, 10, len(result.TransformSteps))

	for i, tf := range transforms {
		mockT := tf.(*mockTransform)
		assert.Equal(t, 1, mockT.applyCount, fmt.Sprintf("transform%d", i))
	}
}

func TestTransformChain_EdgeCase_TransformModifiesRequest(t *testing.T) {
	modifyTransform := &requestModifyTransform{name: "modify"}
	chain := NewTransformChain([]Transform{modifyTransform})

	ctx := &TransformContext{
		Request: &openai.ChatCompletionNewParams{
			Model:       openai.ChatModel("gpt-4"),
			MaxTokens:   param.Opt[int64]{Value: 1024},
			Temperature: param.Opt[float64]{Value: 0.7},
		},
	}

	result, err := chain.Execute(ctx)

	require.NoError(t, err)
	req, ok := result.Request.(*openai.ChatCompletionNewParams)
	assert.True(t, ok)
	assert.Equal(t, int64(2048), req.MaxTokens.Value)
	assert.Equal(t, 0.9, req.Temperature.Value)
}

func TestTransformChain_EdgeCase_NilContext(t *testing.T) {
	chain := NewTransformChain([]Transform{&mockTransform{name: "test"}})
	assert.Panics(t, func() { _, _ = chain.Execute(nil) })
}

func TestTransformChain_EdgeCase_NilTransformInChain(t *testing.T) {
	chain := NewTransformChain([]Transform{&mockTransform{name: "first"}, nil, &mockTransform{name: "second"}})
	ctx := &TransformContext{Request: &openai.ChatCompletionNewParams{}}
	result, err := chain.Execute(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	// Nil transform is ignored, so only "first" and "second" should be in steps
	assert.Equal(t, []string{"first", "second"}, result.TransformSteps)
}

// Error propagation tests
func TestTransformChain_ErrorPropagation_ValidationError(t *testing.T) {
	customValidateTransform := &customValidationErrorTransform{name: "validate"}
	chain := NewTransformChain([]Transform{customValidateTransform})

	ctx := &TransformContext{Request: &openai.ChatCompletionNewParams{}}

	result, err := chain.Execute(ctx)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "validate")
	assert.Contains(t, err.Error(), "validation error")
}

func TestTransformChain_ErrorPropagation_WrappedError(t *testing.T) {
	transforms := []*mockTransform{
		{name: "first"},
		{name: "failing", shouldFail: true, errorMsg: "original error"},
		{name: "third"},
	}

	chain := NewTransformChain([]Transform{transforms[0], transforms[1], transforms[2]})
	ctx := &TransformContext{Request: &openai.ChatCompletionNewParams{}}

	result, err := chain.Execute(ctx)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "'failing' failed")
	assert.Contains(t, err.Error(), "original error")
}
