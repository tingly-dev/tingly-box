package agent

// BuildClaudeCodeEnv constructs environment variables for Claude Code.
// This function contains the business logic for unified vs separate mode.
func BuildClaudeCodeEnv(baseURL, apiKey string, unified bool) map[string]string {
	basePath := baseURL + "/tingly/claude_code"

	env := map[string]string{
		"DISABLE_TELEMETRY":                        "1",
		"DISABLE_ERROR_REPORTING":                  "1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"CLAUDE_CODE_MAX_OUTPUT_TOKENS":            "32000",
		"API_TIMEOUT_MS":                           "3000000",
		"ANTHROPIC_BASE_URL":                       basePath,
		"ANTHROPIC_AUTH_TOKEN":                     apiKey,
	}

	if unified {
		// Unified mode - all point to same model
		env["ANTHROPIC_MODEL"] = "tingly/cc"
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = "tingly/cc"
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = "tingly/cc"
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = "tingly/cc"
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = "tingly/cc"
	} else {
		// Separate mode - different models for different purposes
		env["ANTHROPIC_MODEL"] = "tingly/cc-default"
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = "tingly/cc-haiku"
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = "tingly/cc-opus"
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = "tingly/cc-sonnet"
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = "tingly/cc-subagent"
	}

	return env
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

// CollectCodexModels deduplicates and preserves order of model names.
// This helper processes routing rules to extract unique model names.
func CollectCodexModels(rules []string) []string {
	seen := map[string]struct{}{}
	var out []string

	for _, ruleModel := range rules {
		model := trimSpace(ruleModel)
		if model == "" {
			continue
		}
		if _, exists := seen[model]; exists {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}

	return out
}

// String helpers to avoid importing strings package

func trimSpace(s string) string {
	// Simple trim leading/trailing whitespace
	start := 0
	end := len(s)

	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}

	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}

	return s[start:end]
}
