package agent

import (
	"strings"

	aiagent "github.com/tingly-dev/tingly-box/ai/agent"
	serverconfig "github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// BuildClaudeCodeModelConfig returns the model each Claude Code slot
// requests under the main claude_code routing (see ClaudeCodeSlotModels).
func BuildClaudeCodeModelConfig(cfg *serverconfig.Config) aiagent.ClaudeCodeModelConfig {
	m, _ := ClaudeCodeSlotModels(cfg, string(typ.ScenarioClaudeCode), false)
	return aiagent.ClaudeCodeModelConfig{
		Default:  m["ANTHROPIC_MODEL"],
		Haiku:    m["ANTHROPIC_DEFAULT_HAIKU_MODEL"],
		Sonnet:   m["ANTHROPIC_DEFAULT_SONNET_MODEL"],
		Opus:     m["ANTHROPIC_DEFAULT_OPUS_MODEL"],
		Fable:    m["ANTHROPIC_DEFAULT_FABLE_MODEL"],
		SubAgent: m["CLAUDE_CODE_SUBAGENT_MODEL"],
	}
}

// openCodeModelEntry builds a single OpenCode provider model entry.
//
// OpenCode only shows the image-attachment affordance and forwards image
// content for a custom (non-catalog) provider model when the model entry
// explicitly declares attachment/modalities support — otherwise it defaults
// both to false/text-only (see sst/opencode's provider.ts: capabilities are
// derived from `model.attachment ?? false` and
// `model.modalities?.input?.includes("image") ?? false`). tingly-box has no
// per-model vision-capability data of its own yet, so — mirroring the same
// conservative-but-permissive stance ApplyCodexConfig's RenderCodexModelCatalog
// already takes for Codex (input_modalities always ["text", "image"]) — every
// model tingly-box writes into opencode.json is declared attachment-capable.
func openCodeModelEntry(name string) map[string]interface{} {
	return map[string]interface{}{
		"name":       name,
		"attachment": true,
		"modalities": map[string]interface{}{
			"input":  []string{"text", "image"},
			"output": []string{"text"},
		},
	}
}

// BuildOpenCodeModels builds the OpenCode provider "models" map from a list
// of request-model names, declaring each as attachment/vision-capable (see
// openCodeModelEntry). An empty list falls back to the single default model
// "tingly-opencode" so a fresh setup with no routing rules yet still gets a
// usable config.
func BuildOpenCodeModels(names []string) map[string]interface{} {
	if len(names) == 0 {
		return map[string]interface{}{
			"tingly-opencode": openCodeModelEntry("tingly-opencode"),
		}
	}
	models := make(map[string]interface{}, len(names))
	for _, name := range names {
		models[name] = openCodeModelEntry(name)
	}
	return models
}

// BuildOpenCodeConfig constructs the OpenCode configuration object.
// This function contains the business logic for OpenCode config structure.
func BuildOpenCodeConfig(configBaseURL, apiKey string, models map[string]interface{}) map[string]interface{} {
	if len(models) == 0 {
		// Default single-model layout
		models = BuildOpenCodeModels(nil)
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
		model := strings.TrimSpace(ruleModel)
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
