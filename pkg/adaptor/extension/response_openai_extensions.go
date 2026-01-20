package extension

import (
	"tingly-box/internal/typ"
)

// ResponseTransform applies provider-specific transformations to OpenAI responses
type ResponseTransform func(map[string]interface{}, *typ.Provider, string) map[string]interface{}

// responseConfig maps APIBase patterns to their response transforms
type responseConfig struct {
	APIBasePattern string
	Transform      ResponseTransform
}

// ResponseConfigs holds all registered provider response configurations
var ResponseConfigs = []responseConfig{
	// DeepSeek - ensure reasoning_content is always present
	{"api.deepseek.com", applyDeepSeekResponseTransform},
}

// GetResponseTransform identifies provider by APIBase and returns its response transform
func GetResponseTransform(provider *typ.Provider) ResponseTransform {
	if provider == nil {
		return nil
	}

	apiBase := provider.APIBase
	for _, config := range ResponseConfigs {
		if containsIgnoreCase(apiBase, config.APIBasePattern) {
			return config.Transform
		}
	}

	return nil
}

// ApplyResponseTransforms applies provider-specific transformations to responses
func ApplyResponseTransforms(resp map[string]interface{}, provider *typ.Provider, model string) map[string]interface{} {
	if transform := GetResponseTransform(provider); transform != nil {
		return transform(resp, provider, model)
	}
	return resp
}

// applyDeepSeekResponseTransform ensures reasoning_content is present for DeepSeek
func applyDeepSeekResponseTransform(resp map[string]interface{}, provider *typ.Provider, model string) map[string]interface{} {
	if choices, ok := resp["choices"].([]map[string]interface{}); ok && len(choices) > 0 {
		if message, ok := choices[0]["message"].(map[string]interface{}); ok {
			if _, has := message["reasoning_content"]; !has {
				message["reasoning_content"] = ""
			}
		}
	}
	return resp
}

// containsIgnoreCase checks if a string contains a substring (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			sc := s[i+j]
			ss := substr[j]
			if sc >= 'A' && sc <= 'Z' {
				sc += 32
			}
			if ss >= 'A' && ss <= 'Z' {
				ss += 32
			}
			if sc != ss {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
