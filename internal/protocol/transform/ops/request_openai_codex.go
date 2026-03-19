package ops

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ApplyCodexTransform applies Codex-specific transformations to a Responses API request.
// This is a backward-compatible wrapper for the server-level call.
// Deprecated: Use ApplyCodexResponsesTransform with TransformContext instead.
func ApplyCodexTransform(anthropicReq *anthropic.BetaMessageNewParams, responsesReq *responses.ResponseNewParams) *responses.ResponseNewParams {
	return ApplyCodexResponsesTransform(responsesReq, anthropicReq)
}

// ApplyCodexResponsesTransform applies Codex-specific transformations to a Responses API request.
// This is called from VendorTransform for Codex backend providers.
//
// It handles:
//   - Stream configuration (always enabled for Codex)
//   - Reasoning config (converts Anthropic "thinking" to Codex "reasoning")
//   - Tool name shortening (64 char limit)
//   - Tool parameter normalization
//   - Special tool mappings (web_search_20250305 -> web_search)
//
// Reference: ref/anthropic2codex.go.ref
func ApplyCodexResponsesTransform(req *responses.ResponseNewParams, originalRequest interface{}) *responses.ResponseNewParams {
	// Extract original Anthropic beta request from OriginalRequest
	anthropicReq, ok := originalRequest.(*anthropic.BetaMessageNewParams)
	if !ok {
		// No original Anthropic beta request available, return as-is
		return req
	}

	// Convert responses request to JSON for manipulation
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return req
	}

	result := gjson.ParseBytes(reqJSON)
	template := result.Raw

	// Set stream to true for Codex
	template, _ = sjson.Set(template, "stream", true)
	template, _ = sjson.Set(template, "store", false)

	// Add include for reasoning.encrypted_content if not present
	if !result.Get("include").Exists() || !result.Get("include").IsArray() {
		template, _ = sjson.Set(template, "include", []string{"reasoning.encrypted_content"})
	}

	// Convert reasoning configuration from Anthropic "thinking" to Codex "reasoning"
	// Anthropic beta uses: thinking { OfEnabled/OfAdaptive/OfDisabled }
	// Codex uses: reasoning { effort, summary }
	template = applyCodexThinkingConfig(template, anthropicReq)

	// Ensure parallel_tool_calls is set
	template, _ = sjson.Set(template, "parallel_tool_calls", true)

	// Convert tools - apply Codex-specific transformations
	if len(anthropicReq.Tools) > 0 {
		template = applyCodexToolTransforms(template, anthropicReq, result)
	}

	// Parse back to ResponseNewParams
	var transformed responses.ResponseNewParams
	if err := json.Unmarshal([]byte(template), &transformed); err != nil {
		return req
	}

	return &transformed
}

// applyCodexThinkingConfig converts Anthropic thinking to Codex reasoning config.
func applyCodexThinkingConfig(template string, anthropicReq *anthropic.BetaMessageNewParams) string {
	thinking := anthropicReq.Thinking

	if !param.IsOmitted(thinking.OfEnabled) {
		// Type: enabled with budget_tokens
		budget := thinking.OfEnabled.BudgetTokens
		reasoningEffort := convertBudgetToEffort(int(budget))
		template, _ = sjson.Set(template, "reasoning.effort", reasoningEffort)
		template, _ = sjson.Set(template, "reasoning.summary", "auto")
	} else if !param.IsOmitted(thinking.OfAdaptive) {
		// Type: adaptive
		reasoningEffort := "high"
		if !param.IsOmitted(anthropicReq.OutputConfig) {
			outputConfig := anthropicReq.OutputConfig
			if !param.IsOmitted(outputConfig.Effort) {
				reasoningEffort = string(outputConfig.Effort)
			}
		}
		template, _ = sjson.Set(template, "reasoning.effort", reasoningEffort)
		template, _ = sjson.Set(template, "reasoning.summary", "auto")
	} else if !param.IsOmitted(thinking.OfDisabled) {
		// Type: disabled
		template, _ = sjson.Set(template, "reasoning.effort", "low")
		template, _ = sjson.Set(template, "reasoning.summary", "auto")
	}

	return template
}

// applyCodexToolTransforms applies Codex-specific tool transformations.
func applyCodexToolTransforms(template string, anthropicReq *anthropic.BetaMessageNewParams, result gjson.Result) string {
	// Build short name map from declared tools
	var names []string
	for _, toolUnion := range anthropicReq.Tools {
		if !param.IsOmitted(toolUnion.OfTool) {
			names = append(names, toolUnion.OfTool.Name)
		}
	}
	shortMap := buildShortNameMap(names)

	// Update tool names and normalize parameters in the transformed request
	if tools := result.Get("tools"); tools.Exists() && tools.IsArray() {
		toolsArray := tools.Array()
		for i := 0; i < len(toolsArray); i++ {
			toolPath := "tools." + jsonPathIndex(i)
			toolResult := toolsArray[i]

			// Special handling: map Claude web_search_20250305 tool to Codex web_search
			if toolResult.Get("type").String() == "web_search_20250305" {
				template, _ = sjson.SetRaw(template, toolPath, `{"type":"web_search"}`)
				continue
			}

			// Apply shortened name if needed
			originalName := toolResult.Get("name").String()
			if originalName != "" {
				if short, ok := shortMap[originalName]; ok {
					template, _ = sjson.Set(template, toolPath+".name", short)
				}
			}

			// Normalize tool parameters
			if inputSchema := toolResult.Get("input_schema"); inputSchema.Exists() {
				normalizedParams := normalizeToolParameters(inputSchema.Raw)
				template, _ = sjson.SetRaw(template, toolPath+".input_schema", normalizedParams)
			}

			// Clean up tool properties for Codex compatibility
			template, _ = sjson.Delete(template, toolPath+".input_schema.$schema")
			template, _ = sjson.Delete(template, toolPath+".cache_control")
			template, _ = sjson.Delete(template, toolPath+".defer_loading")
			template, _ = sjson.Set(template, toolPath+".strict", false)
		}
	}

	return template
}

// convertBudgetToEffort converts thinking budget tokens to reasoning effort level.
func convertBudgetToEffort(budget int) string {
	switch {
	case budget <= 1000:
		return "low"
	case budget <= 20000:
		return "medium"
	case budget <= 50000:
		return "high"
	default:
		return "xhigh"
	}
}

// normalizeToolParameters ensures object schemas contain at least an empty properties map.
func normalizeToolParameters(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || !gjson.Valid(raw) {
		return `{"type":"object","properties":{}}`
	}
	schema := raw
	result := gjson.Parse(raw)
	schemaType := result.Get("type").String()
	if schemaType == "" {
		schema, _ = sjson.Set(schema, "type", "object")
		schemaType = "object"
	}
	if schemaType == "object" && !result.Get("properties").Exists() {
		schema, _ = sjson.SetRaw(schema, "properties", `{}`)
	}
	return schema
}

// buildShortNameMap creates a mapping of original tool names to shortened names.
// Ensures uniqueness of shortened names within a request (64 char limit for Codex).
func buildShortNameMap(names []string) map[string]string {
	const limit = 64
	used := map[string]struct{}{}
	m := map[string]string{}

	baseCandidate := func(n string) string {
		if len(n) <= limit {
			return n
		}
		if strings.HasPrefix(n, "mcp__") {
			idx := strings.LastIndex(n, "__")
			if idx > 0 {
				cand := "mcp__" + n[idx+2:]
				if len(cand) > limit {
					cand = cand[:limit]
				}
				return cand
			}
		}
		return n[:limit]
	}

	makeUnique := func(cand string) string {
		if _, ok := used[cand]; !ok {
			return cand
		}
		base := cand
		for i := 1; ; i++ {
			suffix := "_" + fmt.Sprintf("%d", i)
			allowed := limit - len(suffix)
			if allowed < 0 {
				allowed = 0
			}
			tmp := base
			if len(tmp) > allowed {
				tmp = tmp[:allowed]
			}
			tmp = tmp + suffix
			if _, ok := used[tmp]; !ok {
				return tmp
			}
		}
	}

	for _, n := range names {
		cand := baseCandidate(n)
		uniq := makeUnique(cand)
		used[uniq] = struct{}{}
		m[n] = uniq
	}
	return m
}

// shortenNameIfNeeded applies simple shortening rule for a single name.
func shortenNameIfNeeded(name string) string {
	const limit = 64
	if len(name) <= limit {
		return name
	}
	if strings.HasPrefix(name, "mcp__") {
		idx := strings.LastIndex(name, "__")
		if idx > 0 {
			cand := "mcp__" + name[idx+2:]
			if len(cand) > limit {
				return cand[:limit]
			}
			return cand
		}
	}
	return name[:limit]
}

// jsonPathIndex converts a numeric index to JSON path format.
func jsonPathIndex(i int) string {
	return fmt.Sprintf("%d", i)
}
