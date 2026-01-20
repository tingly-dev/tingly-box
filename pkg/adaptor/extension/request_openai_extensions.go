package extension

import (
	"strings"

	"tingly-box/internal/typ"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

// OpenAIConfig contains additional metadata that may be used by provider transforms
type OpenAIConfig struct {
	// HasThinking indicates whether the request contains thinking content
	// This can be used by providers like DeepSeek to handle reasoning_content
	HasThinking bool

	// ReasoningEffort specifies the reasoning effort level for OpenAI-compatible APIs
	// Valid values: "none", "minimal", "low", "medium", "high", "xhigh"
	// Defaults to "low" when HasThinking is true
	ReasoningEffort shared.ReasoningEffort

	// Future fields can be added here as needed for provider-specific transformations
}

// ProviderTransform applies provider-specific transformations to OpenAI requests
type ProviderTransform func(*openai.ChatCompletionNewParams, *typ.Provider, string, *OpenAIConfig) *openai.ChatCompletionNewParams

// providerConfig maps APIBase patterns to their transforms
type providerConfig struct {
	APIBasePattern string
	ModelPattern   string // Optional: if specified, model name must also match this pattern
	Transform      ProviderTransform
}

// ProviderConfigs holds all registered provider configurations
// Add new providers here with their APIBase domain patterns
var ProviderConfigs = []providerConfig{
	// DeepSeek - official API
	{
		APIBasePattern: "api.deepseek.com",
		ModelPattern:   "*", // No specific model pattern needed for DeepSeek official API
		Transform:      applyDeepSeekTransform,
	},

	// Gemini - official Google API
	{
		APIBasePattern: "generativelanguage.googleapis.com",
		ModelPattern:   "gemini", // No specific model pattern needed for official Gemini API
		Transform:      applyGeminiTransform,
	},

	// Gemini - Poe (only for Gemini models)
	{
		APIBasePattern: "poe.com",
		ModelPattern:   "gemini", // Apply transform only if model name contains "gemini"
		Transform:      applyGeminiPoeTransform,
	},

	// Gemini - OpenRouter
	// {"openrouter.ai", applyGeminiOpenRouterTransform},
}

// GetProviderTransform identifies provider by APIBase and returns its transform
// Returns nil if no specific transform is needed (fallback to default)
func GetProviderTransform(provider *typ.Provider, model string) ProviderTransform {
	if provider == nil {
		return nil
	}

	apiBase := strings.ToLower(provider.APIBase)
	modelLower := strings.ToLower(model)

	// Match by APIBase domain and optional ModelPattern
	for _, config := range ProviderConfigs {
		if strings.Contains(apiBase, config.APIBasePattern) {
			// If a model pattern is specified, it must also match
			if config.ModelPattern == "*" || strings.Contains(modelLower, config.ModelPattern) {
				return config.Transform
			}
		}
	}

	// No specific transform needed - use default
	return nil
}

// applyDefaultTransform applies default transformations for OpenAI-compatible requests
// This handles standard fields like reasoning_effort that are widely supported
func applyDefaultTransform(req *openai.ChatCompletionNewParams, config *OpenAIConfig) *openai.ChatCompletionNewParams {
	if config.HasThinking && config.ReasoningEffort != "" {
		// Set reasoning_effort from config for OpenAI-compatible APIs
		// This is widely supported by many providers (OpenAI, Azure, etc.)
		req.ReasoningEffort = config.ReasoningEffort
	} else if config.HasThinking {
		extra := req.ExtraFields()
		if extra == nil {
			extra = map[string]interface{}{
				"thinking": map[string]interface{}{
					"type": "enabled",
				},
			}
		} else {
			extra["thinking"] = map[string]interface{}{
				"type": "enabled",
			}
		}
		req.SetExtraFields(extra)
	}
	return req
}

// ApplyProviderTransforms applies provider-specific transformations
// Falls back to default handling if no specific transform found
func ApplyProviderTransforms(req *openai.ChatCompletionNewParams, provider *typ.Provider, model string, config *OpenAIConfig) *openai.ChatCompletionNewParams {
	if transform := GetProviderTransform(provider, model); transform != nil {
		return transform(req, provider, model, config)
	}
	// Default: apply standard OpenAI-compatible transformations
	return applyDefaultTransform(req, config)
}
