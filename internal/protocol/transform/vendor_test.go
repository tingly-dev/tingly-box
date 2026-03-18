package transform

import (
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform/ops"
)

func TestNewVendorTransform(t *testing.T) {
	vt := NewVendorTransform("api.openai.com")
	assert.Equal(t, "vendor_adjust", vt.Name())
	assert.Equal(t, "api.openai.com", vt.ProviderURL)
}

func TestVendorTransform_Apply_Success(t *testing.T) {
	vt := NewVendorTransform("api.openai.com")

	ctx := &TransformContext{
		Request:     newOpenAIRequest("gpt-4", 1024),
		ProviderURL: "api.openai.com",
		Extra: map[string]interface{}{
			"openaiConfig": &protocol.OpenAIConfig{HasThinking: false, ReasoningEffort: "none"},
		},
	}

	err := vt.Apply(ctx)
	require.NoError(t, err)
	_, ok := ctx.Request.(*openai.ChatCompletionNewParams)
	assert.True(t, ok)
}

func TestVendorTransform_Apply_MissingOpenAIConfig(t *testing.T) {
	vt := NewVendorTransform("api.openai.com")

	ctx := &TransformContext{
		Request:     newOpenAIRequest("gpt-4", 1024),
		ProviderURL: "api.openai.com",
		Extra:       map[string]interface{}{},
	}

	err := vt.Apply(ctx)
	require.NoError(t, err) // Should use default config
}

func TestVendorTransform_Apply_InvalidRequestType(t *testing.T) {
	vt := NewVendorTransform("api.openai.com")

	ctx := &TransformContext{
		Request:     "invalid",
		ProviderURL: "api.openai.com",
		Extra:       map[string]interface{}{},
	}

	err := vt.Apply(ctx)
	require.Error(t, err)

	validationErr, ok := err.(*ValidationError)
	require.True(t, ok)
	assert.Equal(t, "request", validationErr.Field)
}

func TestVendorTransform_Apply_NilRequest(t *testing.T) {
	vt := NewVendorTransform("api.openai.com")

	ctx := &TransformContext{
		Request:     nil,
		ProviderURL: "api.openai.com",
		Extra:       map[string]interface{}{},
	}

	err := vt.Apply(ctx)
	require.Error(t, err)
}

func TestVendorTransform_Protocols(t *testing.T) {
	tests := []struct {
		name        string
		providerURL string
		model       string
		maxTokens   int64
	}{
		{"OpenAI", "api.openai.com", "gpt-4", 1024},
		{"DeepSeek", "api.deepseek.com", "deepseek-chat", 4096},
		{"Moonshot", "api.moonshot.cn", "moonshot-v1-8k", 8192},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vt := NewVendorTransform(tt.providerURL)

			ctx := &TransformContext{
				Request:     newOpenAIRequest(tt.model, tt.maxTokens),
				ProviderURL: tt.providerURL,
				Extra:       map[string]interface{}{"openaiConfig": &protocol.OpenAIConfig{}},
			}

			err := vt.Apply(ctx)
			require.NoError(t, err)
		})
	}
}

func TestNormalizeProviderURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://api.openai.com/v1/chat/completions", "api.openai.com"},
		{"http://api.deepseek.com:8080/v1", "api.deepseek.com"},
		{"api.moonshot.cn", "api.moonshot.cn"},
		{"api.openai.com:443", "api.openai.com"},
		{"https://api.anthropic.com", "api.anthropic.com"},
		{"https://api.openai.com/", "api.openai.com"},
		{"", ""},
		{"  https://api.openai.com  ", "api.openai.com"},
		{"HTTPS://API.OPENAI.COM", "api.openai.com"},
		{"https://api.example.com:8443/v1/models/gpt-4", "api.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeProviderURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestVendorTransform_TransformerIntegration(t *testing.T) {
	// Test integration with transformer package
	providerURL := "api.openai.com"
	req := newOpenAIRequest("gpt-4", 1024)
	config := &protocol.OpenAIConfig{HasThinking: false, ReasoningEffort: "none"}

	// Direct transformer call
	transformed := ops.ApplyProviderTransforms(req, providerURL, "gpt-4", config)
	assert.NotNil(t, transformed)

	// Through VendorTransform
	vt := NewVendorTransform(providerURL)
	ctx := &TransformContext{
		Request:     req,
		ProviderURL: providerURL,
		Extra:       map[string]interface{}{"openaiConfig": config},
	}

	err := vt.Apply(ctx)
	require.NoError(t, err)
	assert.NotNil(t, ctx.Request)
}
