package agent

import (
	aiagent "github.com/tingly-dev/tingly-box/ai/agent"
)

// BuildClaudeCodeModelConfig constructs the model configuration for Claude Code.
// This contains the business logic for unified vs separate mode.
// Exported for use by HTTP handlers.
func BuildClaudeCodeModelConfig(unified bool) aiagent.ClaudeCodeModelConfig {
	if unified {
		return aiagent.ClaudeCodeModelConfig{
			Default: "tingly/cc",
			// All other fields will use Default
		}
	}

	// Separate mode - different models for different purposes
	return aiagent.ClaudeCodeModelConfig{
		Default:  "tingly/cc-default",
		Haiku:    "tingly/cc-haiku",
		Opus:     "tingly/cc-opus",
		Sonnet:   "tingly/cc-sonnet",
		SubAgent: "tingly/cc-subagent",
	}
}

// BuildOpenCodeConfig constructs the OpenCode configuration object.
// This function contains the business logic for OpenCode config structure.
func BuildOpenCodeConfig(configBaseURL, apiKey string, models map[string]interface{}) map[string]interface{} {
	if len(models) == 0 {
		// Default single-model layout
		models = map[string]interface{}{
			"tingly-opencode": map[string]interface{}{"name": "tingly-opencode"},
		}
	}

	providerConfig := map[string]interface{}{
		"tingly-box": map[string]interface{}{
			"name": "tingly-box",
			"npm":  "@ai-sdk/anthropic",
			"options": map[string]interface{}{
				"baseURL": configBaseURL,
				"apiKey":  apiKey,
			},
			"models": models,
		},
	}

	return map[string]interface{}{
		"$schema":  "https://opencode.ai/config.json",
		"provider": providerConfig,
	}
}

