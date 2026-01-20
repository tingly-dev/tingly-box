package extension

import (
	"strings"

	"tingly-box/internal/typ"

	"github.com/openai/openai-go/v3"
)

// OpenAIConfig contains additional metadata that may be used by provider transforms
type OpenAIConfig struct {
	// HasThinking indicates whether the request contains thinking content
	// This can be used by providers like DeepSeek to handle reasoning_content
	HasThinking bool

	// Future fields can be added here as needed for provider-specific transformations
}

// ProviderTransform applies provider-specific transformations to OpenAI requests
type ProviderTransform func(*openai.ChatCompletionNewParams, *typ.Provider, string, *OpenAIConfig) *openai.ChatCompletionNewParams

// providerConfig maps APIBase patterns to their transforms
type providerConfig struct {
	APIBasePattern string
	Transform      ProviderTransform
}

// ProviderConfigs holds all registered provider configurations
// Add new providers here with their APIBase domain patterns
var ProviderConfigs = []providerConfig{
	// DeepSeek - official API
	{"api.deepseek.com", applyDeepSeekTransform},

	// Gemini - official Google API
	{"generativelanguage.googleapis.com", applyGeminiTransform},

	// Gemini - OpenRouter
	{"openrouter.ai", applyGeminiOpenRouterTransform},
}

// GetProviderTransform identifies provider by APIBase and returns its transform
// Returns nil if no specific transform is needed (fallback to default)
func GetProviderTransform(provider *typ.Provider) ProviderTransform {
	if provider == nil {
		return nil
	}

	apiBase := strings.ToLower(provider.APIBase)

	// Match by APIBase domain
	for _, config := range ProviderConfigs {
		if strings.Contains(apiBase, config.APIBasePattern) {
			return config.Transform
		}
	}

	// No specific transform needed - use default
	return nil
}

// ApplyProviderTransforms applies provider-specific transformations
// Falls back to default handling if no specific transform found
func ApplyProviderTransforms(req *openai.ChatCompletionNewParams, provider *typ.Provider, model string, config *OpenAIConfig) *openai.ChatCompletionNewParams {
	if transform := GetProviderTransform(provider); transform != nil {
		return transform(req, provider, model, config)
	}
	// Default: no transformation needed
	return req
}
