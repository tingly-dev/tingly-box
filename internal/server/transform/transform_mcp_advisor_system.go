package transform

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"

	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
)

// advisorInjectedToolName is the normalized name used by ListServerToolsForInjection
// for the built-in advisor virtual tool. Kept here to avoid an import cycle on the
// runtime constant; mirrors NormalizeToolName("builtin", "advisor").
const advisorInjectedToolName = "tingly_box_mcp__builtin__advisor"

// containsAdvisorTool reports whether the given injected MCP tool list contains
// the built-in advisor tool.
func containsAdvisorTool(tools []openai.ChatCompletionToolUnionParam) bool {
	for _, t := range tools {
		if fn := t.GetFunction(); fn != nil && fn.Name == advisorInjectedToolName {
			return true
		}
	}
	return false
}

// appendAdvisorBehaviorToOpenAISystem appends the advisor behavioral contract
// to the worker's system message(s) in an OpenAI Chat Completions request.
// Preserves the original Content variant: string messages are extended in-place,
// array messages get a new text part appended. If no system message exists, one is prepended.
func appendAdvisorBehaviorToOpenAISystem(messages []openai.ChatCompletionMessageParamUnion) []openai.ChatCompletionMessageParamUnion {
	suffix := "\n\n" + runtime.AdvisorBehaviorPrompt

	for i := range messages {
		if sys := messages[i].OfSystem; sys != nil {
			if !param.IsOmitted(sys.Content.OfString) {
				sys.Content.OfString = param.NewOpt(sys.Content.OfString.Value + suffix)
			} else {
				sys.Content.OfArrayOfContentParts = append(
					sys.Content.OfArrayOfContentParts,
					openai.ChatCompletionContentPartTextParam{Text: suffix},
				)
			}
			return messages
		}
		if dev := messages[i].OfDeveloper; dev != nil {
			if !param.IsOmitted(dev.Content.OfString) {
				dev.Content.OfString = param.NewOpt(dev.Content.OfString.Value + suffix)
			} else {
				dev.Content.OfArrayOfContentParts = append(
					dev.Content.OfArrayOfContentParts,
					openai.ChatCompletionContentPartTextParam{Text: suffix},
				)
			}
			return messages
		}
	}

	// No system/developer message present; prepend a fresh one.
	return append([]openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(runtime.AdvisorBehaviorPrompt),
	}, messages...)
}

// appendAdvisorBehaviorToAnthropicV1System appends the advisor behavioral
// contract to the system field of an Anthropic v1 Messages request.
func appendAdvisorBehaviorToAnthropicV1System(system []anthropic.TextBlockParam) []anthropic.TextBlockParam {
	if len(system) == 0 {
		return []anthropic.TextBlockParam{{Text: runtime.AdvisorBehaviorPrompt}}
	}
	system = append(system, anthropic.TextBlockParam{Text: runtime.AdvisorBehaviorPrompt})
	return system
}

// appendAdvisorBehaviorToAnthropicBetaSystem appends the advisor behavioral
// contract to the system field of an Anthropic beta Messages request.
func appendAdvisorBehaviorToAnthropicBetaSystem(system []anthropic.BetaTextBlockParam) []anthropic.BetaTextBlockParam {
	if len(system) == 0 {
		return []anthropic.BetaTextBlockParam{{Text: runtime.AdvisorBehaviorPrompt}}
	}
	system = append(system, anthropic.BetaTextBlockParam{Text: runtime.AdvisorBehaviorPrompt})
	return system
}
