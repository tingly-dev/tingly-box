package request

import (
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicparam "github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
	"google.golang.org/genai"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/thinking"
)

// This file holds the shared core of the Anthropic→OpenAI and
// Anthropic→Google request converters. The overlapping Anthropic v1 and Beta
// SDK fields are structurally equivalent but nominally distinct, which
// historically led to fully duplicated converter pairs (one per type).
// Instead, the Beta adapter maps supported fields into canonical v1 SDK types,
// and a single core performs the actual conversion.

// anthropicRequestView is the canonical Anthropic request subset consumed by
// the shared OpenAI and Google converters. Its fields deliberately use the v1
// SDK types and nesting; Beta adapters bridge nominally distinct SDK types into
// this shape without inventing a parallel request model.
type anthropicRequestView struct {
	Model        anthropic.Model
	MaxTokens    int64
	CacheControl anthropic.CacheControlEphemeralParam
	System       []anthropic.TextBlockParam
	Messages     []anthropic.MessageParam
	Tools        []anthropic.ToolUnionParam
	ToolChoice   anthropic.ToolChoiceUnionParam
	Thinking     anthropic.ThinkingConfigParamUnion
	OutputConfig anthropic.OutputConfigParam
}

func viewAnthropicBetaCacheControl(control *anthropic.BetaCacheControlEphemeralParam) anthropic.CacheControlEphemeralParam {
	if control == nil || anthropicparam.IsOmitted(*control) {
		return anthropic.CacheControlEphemeralParam{}
	}
	return anthropic.CacheControlEphemeralParam{
		Type: control.Type,
		TTL:  anthropic.CacheControlEphemeralTTL(control.TTL),
	}
}

func hasAnthropicCacheControl(control anthropic.CacheControlEphemeralParam) bool {
	return !anthropicparam.IsOmitted(control)
}

func viewAnthropicV1Request(req *anthropic.MessageNewParams) anthropicRequestView {
	return anthropicRequestView{
		Model:        req.Model,
		MaxTokens:    req.MaxTokens,
		CacheControl: req.CacheControl,
		System:       req.System,
		Messages:     req.Messages,
		Tools:        req.Tools,
		ToolChoice:   req.ToolChoice,
		Thinking:     req.Thinking,
		OutputConfig: req.OutputConfig,
	}
}

// ───────────────────────── beta adapters ─────────────────────────

func viewAnthropicBetaBlock(block anthropic.BetaContentBlockParamUnion) anthropic.ContentBlockParamUnion {
	switch {
	case block.OfText != nil:
		return anthropic.ContentBlockParamUnion{OfText: &anthropic.TextBlockParam{
			Text:         block.OfText.Text,
			CacheControl: viewAnthropicBetaCacheControl(&block.OfText.CacheControl),
		}}
	case block.OfThinking != nil:
		return anthropic.ContentBlockParamUnion{OfThinking: &anthropic.ThinkingBlockParam{
			Thinking:  block.OfThinking.Thinking,
			Signature: block.OfThinking.Signature,
		}}
	case block.OfRedactedThinking != nil:
		return anthropic.ContentBlockParamUnion{OfRedactedThinking: &anthropic.RedactedThinkingBlockParam{
			Data: block.OfRedactedThinking.Data,
		}}
	case block.OfToolUse != nil:
		return anthropic.ContentBlockParamUnion{OfToolUse: &anthropic.ToolUseBlockParam{
			ID:           block.OfToolUse.ID,
			Name:         block.OfToolUse.Name,
			Input:        block.OfToolUse.Input,
			CacheControl: viewAnthropicBetaCacheControl(&block.OfToolUse.CacheControl),
		}}
	case block.OfToolResult != nil:
		return anthropic.ContentBlockParamUnion{OfToolResult: &anthropic.ToolResultBlockParam{
			ToolUseID:    block.OfToolResult.ToolUseID,
			IsError:      block.OfToolResult.IsError,
			CacheControl: viewAnthropicBetaCacheControl(&block.OfToolResult.CacheControl),
			Content: []anthropic.ToolResultBlockParamContentUnion{{
				OfText: &anthropic.TextBlockParam{Text: convertBetaToolResultContent(block.OfToolResult.Content)},
			}},
		}}
	case block.OfImage != nil:
		image := &anthropic.ImageBlockParam{
			CacheControl: viewAnthropicBetaCacheControl(&block.OfImage.CacheControl),
		}
		if block.OfImage.Source.OfBase64 != nil {
			image.Source.OfBase64 = &anthropic.Base64ImageSourceParam{
				Data:      block.OfImage.Source.OfBase64.Data,
				MediaType: anthropic.Base64ImageSourceMediaType(block.OfImage.Source.OfBase64.MediaType),
			}
		} else if block.OfImage.Source.OfURL != nil {
			image.Source.OfURL = &anthropic.URLImageSourceParam{
				URL: block.OfImage.Source.OfURL.URL,
			}
		}
		return anthropic.ContentBlockParamUnion{OfImage: image}
	}
	return anthropic.ContentBlockParamUnion{}
}

func viewAnthropicBetaMessage(msg anthropic.BetaMessageParam) anthropic.MessageParam {
	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(msg.Content))
	for _, block := range msg.Content {
		blocks = append(blocks, viewAnthropicBetaBlock(block))
	}
	return anthropic.MessageParam{
		Role:    anthropic.MessageParamRole(msg.Role),
		Content: blocks,
	}
}

func viewAnthropicBetaTools(tools []anthropic.BetaToolUnionParam) []anthropic.ToolUnionParam {
	if len(tools) == 0 {
		return nil
	}
	out := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		tool := t.OfTool
		if tool == nil {
			out = append(out, anthropic.ToolUnionParam{})
			continue
		}
		out = append(out, anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{
			Name:                tool.Name,
			Description:         tool.Description,
			InputSchema:         viewAnthropicBetaToolInputSchema(tool.InputSchema),
			CacheControl:        viewAnthropicBetaCacheControl(&tool.CacheControl),
			EagerInputStreaming: tool.EagerInputStreaming,
			DeferLoading:        tool.DeferLoading,
			Strict:              tool.Strict,
			Type:                anthropic.ToolType(tool.Type),
			AllowedCallers:      tool.AllowedCallers,
			InputExamples:       tool.InputExamples,
		}})
	}
	return out
}

func viewAnthropicBetaToolInputSchema(schema anthropic.BetaToolInputSchemaParam) anthropic.ToolInputSchemaParam {
	return anthropic.ToolInputSchemaParam{
		Type:        schema.Type,
		Properties:  schema.Properties,
		Required:    schema.Required,
		ExtraFields: schema.ExtraFields,
	}
}

func viewAnthropicBetaToolChoice(tc *anthropic.BetaToolChoiceUnionParam) anthropic.ToolChoiceUnionParam {
	if tc == nil {
		return anthropic.ToolChoiceUnionParam{}
	}
	switch {
	case tc.OfAuto != nil:
		return anthropic.ToolChoiceUnionParam{OfAuto: &anthropic.ToolChoiceAutoParam{
			DisableParallelToolUse: tc.OfAuto.DisableParallelToolUse,
		}}
	case tc.OfAny != nil:
		return anthropic.ToolChoiceUnionParam{OfAny: &anthropic.ToolChoiceAnyParam{
			DisableParallelToolUse: tc.OfAny.DisableParallelToolUse,
		}}
	case tc.OfTool != nil:
		return anthropic.ToolChoiceUnionParam{OfTool: &anthropic.ToolChoiceToolParam{
			Name:                   tc.OfTool.Name,
			DisableParallelToolUse: tc.OfTool.DisableParallelToolUse,
		}}
	case tc.OfNone != nil:
		none := anthropic.NewToolChoiceNoneParam()
		return anthropic.ToolChoiceUnionParam{OfNone: &none}
	}
	return anthropic.ToolChoiceUnionParam{}
}

func viewAnthropicBetaThinking(thinking anthropic.BetaThinkingConfigParamUnion) anthropic.ThinkingConfigParamUnion {
	switch {
	case thinking.OfEnabled != nil:
		return anthropic.ThinkingConfigParamUnion{OfEnabled: &anthropic.ThinkingConfigEnabledParam{
			BudgetTokens: thinking.OfEnabled.BudgetTokens,
			Display:      anthropic.ThinkingConfigEnabledDisplay(thinking.OfEnabled.Display),
		}}
	case thinking.OfDisabled != nil:
		disabled := anthropic.NewThinkingConfigDisabledParam()
		return anthropic.ThinkingConfigParamUnion{OfDisabled: &disabled}
	case thinking.OfAdaptive != nil:
		return anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{
			Display: anthropic.ThinkingConfigAdaptiveDisplay(thinking.OfAdaptive.Display),
		}}
	}
	return anthropic.ThinkingConfigParamUnion{}
}

func viewAnthropicBetaRequest(req *anthropic.BetaMessageNewParams) anthropicRequestView {
	view := anthropicRequestView{
		Model:        req.Model,
		MaxTokens:    req.MaxTokens,
		CacheControl: viewAnthropicBetaCacheControl(&req.CacheControl),
		Tools:        viewAnthropicBetaTools(req.Tools),
		ToolChoice:   viewAnthropicBetaToolChoice(&req.ToolChoice),
		Thinking:     viewAnthropicBetaThinking(req.Thinking),
		OutputConfig: anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffort(req.OutputConfig.Effort),
		},
	}
	for _, sys := range req.System {
		view.System = append(view.System, anthropic.TextBlockParam{
			Text:         sys.Text,
			CacheControl: viewAnthropicBetaCacheControl(&sys.CacheControl),
		})
	}
	for _, msg := range req.Messages {
		view.Messages = append(view.Messages, viewAnthropicBetaMessage(msg))
	}
	return view
}

// ───────────────────────── OpenAI core ─────────────────────────

// convertAnthropicViewToOpenAIRequest is the shared Anthropic→OpenAI request
// conversion, operating on the normalized view.
func convertAnthropicViewToOpenAIRequest(view anthropicRequestView, isStreaming bool, disableStreamUsage bool) (*openai.ChatCompletionNewParams, *protocol.OpenAIConfig) {
	openaiReq := &openai.ChatCompletionNewParams{
		Model: openai.ChatModel(view.Model),
	}

	// Set MaxTokens
	openaiReq.MaxTokens = openai.Opt(view.MaxTokens)

	// Convert messages
	for _, msg := range view.Messages {
		switch msg.Role {
		case anthropic.MessageParamRoleUser:
			// User messages may contain tool_result blocks - need special handling
			openaiReq.Messages = append(openaiReq.Messages, convertAnthropicViewUserToOpenAI(msg.Content)...)
		case anthropic.MessageParamRoleAssistant:
			// Convert assistant message with potential tool_use blocks
			openaiReq.Messages = append(openaiReq.Messages, convertAnthropicViewAssistantToOpenAI(msg.Content))
		}
	}

	// Convert system messages. Keep block boundaries when cache controls are
	// present so their exact prompt prefix survives A→O→A gateway chaining.
	if len(view.System) > 0 {
		var systemMsg openai.ChatCompletionMessageParamUnion
		if systemBlocksHaveCacheControl(view.System) {
			parts := make([]openai.ChatCompletionContentPartTextParam, 0, len(view.System))
			for _, system := range view.System {
				part := openai.ChatCompletionContentPartTextParam{Text: system.Text}
				if hasAnthropicCacheControl(system.CacheControl) {
					part.PromptCacheBreakpoint = openai.NewChatCompletionContentPartTextPromptCacheBreakpointParam()
				}
				parts = append(parts, part)
			}
			systemMsg = openai.SystemMessage(parts)
		} else {
			var systemText strings.Builder
			for _, system := range view.System {
				systemText.WriteString(system.Text)
			}
			systemMsg = openai.SystemMessage(systemText.String())
		}
		// Add system message at the beginning
		openaiReq.Messages = append([]openai.ChatCompletionMessageParamUnion{systemMsg}, openaiReq.Messages...)
	}

	// Convert tools from Anthropic format to OpenAI format
	if len(view.Tools) > 0 {
		openaiReq.Tools = convertAnthropicToolViewsToOpenAI(view.Tools)
		// Convert tool choice
		openaiReq.ToolChoice = convertAnthropicToolChoiceViewToOpenAI(view.ToolChoice)
	}

	hasRepresentableCacheControl := viewHasRepresentableCacheControl(view)
	hasFallbackCacheControl := viewHasToolDefinitionCacheControl(view) || viewHasToolUseCacheControl(view)
	if hasAnthropicCacheControl(view.CacheControl) || hasRepresentableCacheControl || hasFallbackCacheControl {
		if hasAnthropicCacheControl(view.CacheControl) {
			openaiReq.PromptCacheOptions.Mode = "implicit"
		} else {
			openaiReq.PromptCacheOptions.Mode = "explicit"
		}
		if !hasRepresentableCacheControl && hasFallbackCacheControl {
			// OpenAI cannot put a breakpoint on a tool definition or tool call.
			// Advance that boundary to the first cacheable content block, which
			// avoids dropping caching entirely when gateways are chained.
			applyFirstOpenAICacheBreakpoint(openaiReq)
		}
	}

	// thinking
	config := &protocol.OpenAIConfig{
		HasThinking:     false,
		ReasoningEffort: "medium", // Default to "medium" for OpenAI-compatible APIs
	}
	if view.Thinking.OfEnabled != nil || view.Thinking.OfAdaptive != nil || messagesHaveThinking(view.Messages) {
		config.HasThinking = true
		config.ReasoningEffort = "medium"
	}
	if view.Thinking.OfEnabled != nil && view.Thinking.OfEnabled.BudgetTokens > 0 {
		// Tier the explicit budget onto the effort ladder instead of flattening
		// every budget to "medium" (a 32K ultrathink budget is not "medium").
		config.ReasoningEffort = shared.ReasoningEffort(thinking.EffortFromBudget(view.Thinking.OfEnabled.BudgetTokens))
	}
	if view.OutputConfig.Effort != "" {
		// An explicit effort level wins over the budget-derived tier.
		config.ReasoningEffort = shared.ReasoningEffort(view.OutputConfig.Effort)
	}

	// Only set stream_options for streaming requests (per OpenAI API spec)
	if isStreaming && !disableStreamUsage {
		openaiReq.StreamOptions.IncludeUsage = param.Opt[bool]{Value: true}
	}
	return openaiReq, config
}

// convertAnthropicViewAssistantToOpenAI converts an assistant message's
// blocks to a single OpenAI assistant message. Thinking content is preserved
// in the "x_thinking" extra field for provider-specific transforms.
func convertAnthropicViewAssistantToOpenAI(blocks []anthropic.ContentBlockParamUnion) openai.ChatCompletionMessageParamUnion {
	preserveTextParts := blocksHaveCacheControl(blocks)
	var textContent strings.Builder
	var textParts []openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion
	var toolCalls []openai.ChatCompletionMessageToolCallUnionParam
	var thinking string

	for _, block := range blocks {
		switch {
		case block.OfText != nil:
			if preserveTextParts {
				part := openAITextPart(block.OfText.Text, hasAnthropicCacheControl(block.OfText.CacheControl))
				textParts = append(textParts, openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion{
					OfText: &part,
				})
			} else {
				textContent.WriteString(block.OfText.Text)
			}
		case block.OfToolUse != nil:
			// Convert tool_use block to OpenAI tool_call format;
			// marshal input to a JSON string for OpenAI
			var args string
			if argsBytes, err := json.Marshal(block.OfToolUse.Input); err == nil {
				args = string(argsBytes)
			}
			toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: block.OfToolUse.ID,
					Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      block.OfToolUse.Name,
						Arguments: args,
					},
				},
			})
		case block.OfThinking != nil:
			thinking = block.OfThinking.Thinking
		}
	}

	// Build the message directly from typed params — no JSON round-trip.
	assistant := &openai.ChatCompletionAssistantMessageParam{
		ToolCalls: toolCalls,
	}
	if preserveTextParts {
		assistant.Content.OfArrayOfContentParts = textParts
	} else {
		assistant.Content.OfString = openai.Opt(textContent.String())
	}

	// Preserve x_thinking in ExtraFields for provider transforms (e.g., DeepSeek/Moonshot)
	// Must set on OfAssistant (variant level), not on union level, because
	// MarshalUnion only serializes the active variant — union-level ExtraFields are dropped.
	assistant.SetExtraFields(map[string]any{"x_thinking": thinking})

	return openai.ChatCompletionMessageParamUnion{OfAssistant: assistant}
}

// convertAnthropicViewUserToOpenAI converts a user message's blocks to OpenAI
// messages. tool_result blocks become separate role="tool" messages, image
// blocks turn the message into a multimodal content-part array.
func convertAnthropicViewUserToOpenAI(blocks []anthropic.ContentBlockParamUnion) []openai.ChatCompletionMessageParamUnion {
	var result []openai.ChatCompletionMessageParamUnion
	var hasToolResult, hasImage, hasCache bool

	for _, block := range blocks {
		switch {
		case block.OfToolResult != nil:
			hasToolResult = true
		case block.OfImage != nil:
			hasImage = true
		}
		hasCache = hasCache || blockHasCacheControl(block)
	}

	switch {
	case hasToolResult:
		// When there are tool_result blocks, we need to create separate messages
		var textBlocks []anthropic.ContentBlockParamUnion
		for _, block := range blocks {
			switch {
			case block.OfText != nil:
				textBlocks = append(textBlocks, block)
			case block.OfToolResult != nil:
				// Convert tool_result to OpenAI role="tool" message.
				// Truncate tool_call_id to meet OpenAI's 40 character limit.
				resultText := convertToolResultContent(block.OfToolResult.Content)
				if hasAnthropicCacheControl(block.OfToolResult.CacheControl) {
					part := openAITextPart(resultText, true)
					result = append(result, openai.ChatCompletionMessageParamUnion{
						OfTool: &openai.ChatCompletionToolMessageParam{
							ToolCallID: truncateToolCallID(block.OfToolResult.ToolUseID),
							Content: openai.ChatCompletionToolMessageParamContentUnion{
								OfArrayOfContentParts: []openai.ChatCompletionContentPartTextParam{part},
							},
						},
					})
				} else {
					result = append(result, openai.ToolMessage(resultText, truncateToolCallID(block.OfToolResult.ToolUseID)))
				}
			}
		}
		// If there was text content alongside tool results, add it as a user message
		if len(textBlocks) > 0 {
			result = append(result, convertAnthropicViewUserToOpenAI(textBlocks)...)
		}
	case hasImage || hasCache:
		// Multimodal user message: emit an array of text + image_url content parts
		parts := make([]openai.ChatCompletionContentPartUnionParam, 0, len(blocks))
		for _, block := range blocks {
			switch {
			case block.OfText != nil:
				part := openAITextPart(block.OfText.Text, hasAnthropicCacheControl(block.OfText.CacheControl))
				parts = append(parts, openai.ChatCompletionContentPartUnionParam{OfText: &part})
			case block.OfImage != nil:
				url := anthropicImageToOpenAIURL(block.OfImage)
				if url == "" {
					continue
				}
				imagePart := openai.ChatCompletionContentPartImageParam{
					ImageURL: openai.ChatCompletionContentPartImageImageURLParam{URL: url},
				}
				if hasAnthropicCacheControl(block.OfImage.CacheControl) {
					imagePart.PromptCacheBreakpoint = openai.NewChatCompletionContentPartImagePromptCacheBreakpointParam()
				}
				parts = append(parts, openai.ChatCompletionContentPartUnionParam{OfImageURL: &imagePart})
			}
		}
		if len(parts) > 0 {
			result = append(result, openai.UserMessage(parts))
		}
	default:
		// Simple text-only user message
		var textContent strings.Builder
		for _, block := range blocks {
			if block.OfText != nil {
				textContent.WriteString(block.OfText.Text)
			}
		}
		if textContent.Len() > 0 {
			result = append(result, openai.UserMessage(textContent.String()))
		}
	}

	return result
}

func openAITextPart(text string, cacheControl bool) openai.ChatCompletionContentPartTextParam {
	part := openai.ChatCompletionContentPartTextParam{Text: text}
	if cacheControl {
		part.PromptCacheBreakpoint = openai.NewChatCompletionContentPartTextPromptCacheBreakpointParam()
	}
	return part
}

func systemBlocksHaveCacheControl(blocks []anthropic.TextBlockParam) bool {
	for _, block := range blocks {
		if hasAnthropicCacheControl(block.CacheControl) {
			return true
		}
	}
	return false
}

func blockHasCacheControl(block anthropic.ContentBlockParamUnion) bool {
	control := block.GetCacheControl()
	return control != nil && hasAnthropicCacheControl(*control)
}

func blocksHaveCacheControl(blocks []anthropic.ContentBlockParamUnion) bool {
	for _, block := range blocks {
		if blockHasCacheControl(block) &&
			(block.OfText != nil || block.OfImage != nil || block.OfToolResult != nil) {
			return true
		}
	}
	return false
}

func messagesHaveThinking(messages []anthropic.MessageParam) bool {
	for _, message := range messages {
		for _, block := range message.Content {
			if block.OfThinking != nil {
				return true
			}
		}
	}
	return false
}

func viewHasRepresentableCacheControl(view anthropicRequestView) bool {
	if systemBlocksHaveCacheControl(view.System) {
		return true
	}
	for _, message := range view.Messages {
		if blocksHaveCacheControl(message.Content) {
			return true
		}
	}
	return false
}

func viewHasToolDefinitionCacheControl(view anthropicRequestView) bool {
	for _, tool := range view.Tools {
		if tool.OfTool != nil && hasAnthropicCacheControl(tool.OfTool.CacheControl) {
			return true
		}
	}
	return false
}

func viewHasToolUseCacheControl(view anthropicRequestView) bool {
	for _, message := range view.Messages {
		for _, block := range message.Content {
			if block.OfToolUse != nil && hasAnthropicCacheControl(block.OfToolUse.CacheControl) {
				return true
			}
		}
	}
	return false
}

func applyFirstOpenAICacheBreakpoint(req *openai.ChatCompletionNewParams) {
	for i := range req.Messages {
		msg := &req.Messages[i]
		switch {
		case msg.OfSystem != nil:
			if text := msg.OfSystem.Content.OfString.Value; text != "" {
				msg.OfSystem.Content.OfString = param.Opt[string]{}
				msg.OfSystem.Content.OfArrayOfContentParts = []openai.ChatCompletionContentPartTextParam{
					openAITextPart(text, true),
				}
				return
			}
			if len(msg.OfSystem.Content.OfArrayOfContentParts) > 0 {
				msg.OfSystem.Content.OfArrayOfContentParts[0].PromptCacheBreakpoint =
					openai.NewChatCompletionContentPartTextPromptCacheBreakpointParam()
				return
			}
		case msg.OfUser != nil:
			if text := msg.OfUser.Content.OfString.Value; text != "" {
				msg.OfUser.Content.OfString = param.Opt[string]{}
				part := openAITextPart(text, true)
				msg.OfUser.Content.OfArrayOfContentParts = []openai.ChatCompletionContentPartUnionParam{{OfText: &part}}
				return
			}
			for j := range msg.OfUser.Content.OfArrayOfContentParts {
				part := &msg.OfUser.Content.OfArrayOfContentParts[j]
				if part.OfText != nil {
					part.OfText.PromptCacheBreakpoint = openai.NewChatCompletionContentPartTextPromptCacheBreakpointParam()
					return
				}
				if part.OfImageURL != nil {
					part.OfImageURL.PromptCacheBreakpoint = openai.NewChatCompletionContentPartImagePromptCacheBreakpointParam()
					return
				}
			}
		}
	}
}

// anthropicImageToOpenAIURL renders an Anthropic image block as the URL string OpenAI's
// image_url content part expects. Base64 sources become a data: URL; URL
// sources are passed through. Returns "" for unsupported sources.
func anthropicImageToOpenAIURL(image *anthropic.ImageBlockParam) string {
	if image.Source.OfBase64 != nil {
		return "data:" + string(image.Source.OfBase64.MediaType) + ";base64," + image.Source.OfBase64.Data
	}
	if image.Source.OfURL != nil {
		return image.Source.OfURL.URL
	}
	return ""
}

// convertAnthropicToolViewsToOpenAI converts normalized tool definitions to
// OpenAI function tools.
func convertAnthropicToolViewsToOpenAI(tools []anthropic.ToolUnionParam) []openai.ChatCompletionToolUnionParam {
	// nil means the request declared no tools at all; a non-nil empty view
	// slice (tools declared, none convertible — e.g. only server tools)
	// must keep producing "tools": [] on the wire, exactly as before.
	if len(tools) == 0 {
		return nil
	}

	out := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
	for _, union := range tools {
		tool := union.OfTool
		if tool == nil {
			continue
		}
		// Convert Anthropic input schema to OpenAI function parameters
		parameters := convertAnthropicInputSchemaToOpenAIParameters(tool.InputSchema.Properties, tool.InputSchema.Required)

		out = append(out, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        tool.Name,
			Description: param.Opt[string]{Value: tool.Description.Value},
			Parameters:  parameters,
		}))
	}
	return out
}

// convertAnthropicToolChoiceViewToOpenAI converts a normalized tool_choice to
// OpenAI format. Anthropic's "any" (required) maps to auto, as OpenAI has no
// direct equivalent.
func convertAnthropicToolChoiceViewToOpenAI(tc anthropic.ToolChoiceUnionParam) openai.ChatCompletionToolChoiceOptionUnionParam {
	if tc.OfTool != nil {
		return openai.ToolChoiceOptionFunctionToolChoice(
			openai.ChatCompletionNamedToolChoiceFunctionParam{
				Name: tc.OfTool.Name,
			},
		)
	}
	// auto, any, and the default all map to auto
	return openai.ChatCompletionToolChoiceOptionUnionParam{
		OfAuto: openai.Opt("auto"),
	}
}

// ───────────────────────── Google core ─────────────────────────

// convertAnthropicViewToGoogleRequest is the shared Anthropic→Google request
// conversion, operating on the normalized view.
func convertAnthropicViewToGoogleRequest(view anthropicRequestView) (string, []*genai.Content, *genai.GenerateContentConfig) {
	contents := make([]*genai.Content, 0, len(view.Messages))
	config := &genai.GenerateContentConfig{}

	// Set max_tokens
	config.MaxOutputTokens = int32(view.MaxTokens)

	// Convert system message (joined with newlines)
	if len(view.System) > 0 {
		var systemText strings.Builder
		for _, system := range view.System {
			systemText.WriteString(system.Text)
			systemText.WriteString("\n")
		}
		config.SystemInstruction = &genai.Content{
			Role:  "system",
			Parts: []*genai.Part{genai.NewPartFromText(systemText.String())},
		}
	}

	// Convert messages
	for _, msg := range view.Messages {
		if content := convertAnthropicViewMessageToGoogle(msg); content != nil {
			contents = append(contents, content)
		}
	}

	// Convert tools from Anthropic format to Google format
	if len(view.Tools) > 0 {
		config.Tools = []*genai.Tool{
			{
				FunctionDeclarations: convertAnthropicToolViewsToGoogle(view.Tools),
			},
		}
	}

	// Convert tool choice
	if view.ToolChoice.OfAuto != nil || view.ToolChoice.OfTool != nil || view.ToolChoice.OfAny != nil {
		config.ToolConfig = convertAnthropicToolChoiceViewToGoogle(view.ToolChoice)
	}

	return string(view.Model), contents, config
}

// convertAnthropicViewMessageToGoogle converts one normalized message to a
// Google content. Returns nil when the message produces no parts or has an
// unsupported role.
func convertAnthropicViewMessageToGoogle(msg anthropic.MessageParam) *genai.Content {
	switch msg.Role {
	case anthropic.MessageParamRoleUser:
		content := &genai.Content{
			Role:  "user",
			Parts: []*genai.Part{},
		}
		for _, block := range msg.Content {
			switch {
			case block.OfText != nil:
				content.Parts = append(content.Parts, genai.NewPartFromText(block.OfText.Text))
			case block.OfImage != nil:
				// For Google API, images need to be passed as inline data with MIME type
				if block.OfImage.Source.OfBase64 != nil {
					content.Parts = append(content.Parts, &genai.Part{
						InlineData: &genai.Blob{
							MIMEType: string(block.OfImage.Source.OfBase64.MediaType),
							Data:     []byte(block.OfImage.Source.OfBase64.Data),
						},
					})
				} else if block.OfImage.Source.OfURL != nil {
					// For URL images, we'd need to fetch them first
					// For now, skip or handle as text reference
					content.Parts = append(content.Parts, genai.NewPartFromText("[Image: "+block.OfImage.Source.OfURL.URL+"]"))
				}
			case block.OfToolResult != nil:
				// Convert tool_result to function_response.
				// FunctionResponse.Name should be the tool_use ID for Google API.
				// Try to parse as JSON first; if it fails, wrap as plain text output.
				resultText := convertToolResultContent(block.OfToolResult.Content)
				var response map[string]any
				if err := json.Unmarshal([]byte(resultText), &response); err != nil {
					// Not valid JSON, wrap in "output" key
					response = map[string]any{"output": resultText}
				}
				content.Parts = append(content.Parts, &genai.Part{
					FunctionResponse: &genai.FunctionResponse{
						Name:     block.OfToolResult.ToolUseID, // Use tool_use ID as Name
						Response: response,
					},
				})
			case block.OfThinking != nil, block.OfRedactedThinking != nil:
				// Skip thinking blocks - Google API doesn't support them
			}
		}
		if len(content.Parts) == 0 {
			return nil
		}
		return content

	case anthropic.MessageParamRoleAssistant:
		content := &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{},
		}
		for _, block := range msg.Content {
			switch {
			case block.OfText != nil:
				content.Parts = append(content.Parts, genai.NewPartFromText(block.OfText.Text))
			case block.OfToolUse != nil:
				// Convert tool_use to function_call
				var argsInput map[string]interface{}
				if inputBytes, ok := block.OfToolUse.Input.([]byte); ok {
					_ = json.Unmarshal(inputBytes, &argsInput)
				}
				content.Parts = append(content.Parts, &genai.Part{
					FunctionCall: &genai.FunctionCall{
						ID:   block.OfToolUse.ID,
						Name: block.OfToolUse.Name,
						Args: argsInput,
					},
				})
			case block.OfThinking != nil, block.OfRedactedThinking != nil:
				// Skip thinking blocks - Google API doesn't support them
			}
		}
		if len(content.Parts) == 0 {
			return nil
		}
		return content
	}
	return nil
}

// convertAnthropicToolViewsToGoogle converts normalized tool definitions to
// Google function declarations.
func convertAnthropicToolViewsToGoogle(tools []anthropic.ToolUnionParam) []*genai.FunctionDeclaration {
	// nil means no tools declared; non-nil empty keeps the previous
	// empty-declarations shape for requests whose tools were all filtered.
	if len(tools) == 0 {
		return nil
	}

	out := make([]*genai.FunctionDeclaration, 0, len(tools))
	for _, union := range tools {
		tool := union.OfTool
		if tool == nil {
			continue
		}
		// Convert Anthropic input schema to Google parameters
		var parameters *genai.Schema
		if tool.InputSchema.Properties != nil {
			if schemaBytes, err := json.Marshal(tool.InputSchema); err == nil {
				_ = json.Unmarshal(schemaBytes, &parameters)
				// Normalize schema types from lowercase (JSON Schema) to uppercase (Google format)
				NormalizeSchemaTypes(parameters)
			}
		}

		out = append(out, &genai.FunctionDeclaration{
			Name:        tool.Name,
			Description: tool.Description.Value,
			Parameters:  parameters,
		})
	}
	return out
}

// convertAnthropicToolChoiceViewToGoogle converts a normalized tool_choice to
// a Google tool config.
func convertAnthropicToolChoiceViewToGoogle(tc anthropic.ToolChoiceUnionParam) *genai.ToolConfig {
	config := &genai.ToolConfig{
		FunctionCallingConfig: &genai.FunctionCallingConfig{},
	}

	if tc.OfAuto != nil {
		config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAuto
	}
	if tc.OfTool != nil {
		config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAny
		config.FunctionCallingConfig.AllowedFunctionNames = []string{tc.OfTool.Name}
	}
	if tc.OfAny != nil {
		config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAny
	}

	// Default to auto
	if config.FunctionCallingConfig.Mode == "" {
		config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAuto
	}
	return config
}
