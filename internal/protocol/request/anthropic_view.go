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
)

// This file holds the shared core of the Anthropic→OpenAI and
// Anthropic→Google request converters. The Anthropic v1 and Beta SDK types
// are structurally identical but nominally distinct, which historically led
// to fully duplicated converter pairs (one per type). Instead of duplicating
// the conversion logic, each variant is adapted into the normalized views
// below by a thin, mechanical field-copy adapter, and a single core performs
// the actual conversion.

// anthropicBlockKind enumerates the content block variants the converters
// care about.
type anthropicBlockKind int

const (
	blockViewSkip anthropicBlockKind = iota
	blockViewText
	blockViewThinking
	blockViewRedactedThinking
	blockViewToolUse
	blockViewToolResult
	blockViewImage
)

// anthropicBlockView is a normalized view of a v1/beta content block.
type anthropicBlockView struct {
	Kind anthropicBlockKind

	// Text carries the text for blockViewText and the thinking text for
	// blockViewThinking.
	Text string

	// tool_use fields
	ToolID    string
	ToolName  string
	ToolInput any

	// tool_result fields
	ToolResultID   string
	ToolResultText string

	// image fields: base64 sources set MediaType+Data, URL sources set URL.
	ImageMediaType string
	ImageData      string
	ImageURL       string

	// CacheControl marks the end of a reusable prompt prefix. OpenAI Chat has
	// the same concept under prompt_cache_breakpoint.
	CacheControl anthropic.CacheControlEphemeralParam
}

// anthropicMessageView is a normalized view of a v1/beta message.
type anthropicMessageView struct {
	Role   string
	Blocks []anthropicBlockView
}

// anthropicToolView is a normalized view of a v1/beta tool definition.
type anthropicToolView struct {
	Name         string
	Description  string
	CacheControl anthropic.CacheControlEphemeralParam
	// InputSchema is the full schema param value (marshalled for Google).
	InputSchema any
	// Properties/Required are the schema fields (used for OpenAI parameters).
	Properties    any
	Required      []string
	HasProperties bool
}

// anthropicToolChoiceView is a normalized view of a v1/beta tool_choice.
type anthropicToolChoiceView struct {
	HasAuto  bool
	HasAny   bool
	HasTool  bool
	ToolName string
}

// anthropicRequestView is a normalized view of a full v1/beta request.
type anthropicRequestView struct {
	Model        string
	MaxTokens    int64
	CacheControl anthropic.CacheControlEphemeralParam
	System       []anthropic.TextBlockParam
	Messages     []anthropicMessageView
	Tools        []anthropicToolView
	HasTools     bool
	ToolChoice   anthropicToolChoiceView
	// ThinkingEnabled reflects Thinking.OfEnabled/OfAdaptive or the
	// model-level IsThinkingEnabled* check.
	ThinkingEnabled bool
	OutputEffort    string
}

func viewAnthropicV1CacheControl(control *anthropic.CacheControlEphemeralParam) anthropic.CacheControlEphemeralParam {
	if control == nil || anthropicparam.IsOmitted(*control) {
		return anthropic.CacheControlEphemeralParam{}
	}
	return *control
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

// ───────────────────────── v1 adapters ─────────────────────────

func viewAnthropicV1Block(block anthropic.ContentBlockParamUnion) anthropicBlockView {
	cacheControl := block.GetCacheControl()
	cache := viewAnthropicV1CacheControl(cacheControl)
	switch {
	case block.OfText != nil:
		return anthropicBlockView{Kind: blockViewText, Text: block.OfText.Text, CacheControl: cache}
	case block.OfThinking != nil:
		return anthropicBlockView{Kind: blockViewThinking, Text: block.OfThinking.Thinking}
	case block.OfRedactedThinking != nil:
		return anthropicBlockView{Kind: blockViewRedactedThinking}
	case block.OfToolUse != nil:
		return anthropicBlockView{
			Kind:         blockViewToolUse,
			ToolID:       block.OfToolUse.ID,
			ToolName:     block.OfToolUse.Name,
			ToolInput:    block.OfToolUse.Input,
			CacheControl: cache,
		}
	case block.OfToolResult != nil:
		return anthropicBlockView{
			Kind:           blockViewToolResult,
			ToolResultID:   block.OfToolResult.ToolUseID,
			ToolResultText: convertToolResultContent(block.OfToolResult.Content),
			CacheControl:   cache,
		}
	case block.OfImage != nil:
		v := anthropicBlockView{Kind: blockViewImage, CacheControl: cache}
		if block.OfImage.Source.OfBase64 != nil {
			v.ImageMediaType = string(block.OfImage.Source.OfBase64.MediaType)
			v.ImageData = block.OfImage.Source.OfBase64.Data
		} else if block.OfImage.Source.OfURL != nil {
			v.ImageURL = block.OfImage.Source.OfURL.URL
		}
		return v
	}
	return anthropicBlockView{Kind: blockViewSkip}
}

func viewAnthropicV1Message(msg anthropic.MessageParam) anthropicMessageView {
	blocks := make([]anthropicBlockView, 0, len(msg.Content))
	for _, block := range msg.Content {
		blocks = append(blocks, viewAnthropicV1Block(block))
	}
	return anthropicMessageView{Role: string(msg.Role), Blocks: blocks}
}

func viewAnthropicV1Tools(tools []anthropic.ToolUnionParam) []anthropicToolView {
	if len(tools) == 0 {
		return nil
	}
	out := make([]anthropicToolView, 0, len(tools))
	for _, t := range tools {
		tool := t.OfTool
		if tool == nil {
			continue
		}
		out = append(out, anthropicToolView{
			Name:          tool.Name,
			Description:   tool.Description.Value,
			InputSchema:   tool.InputSchema,
			Properties:    tool.InputSchema.Properties,
			Required:      tool.InputSchema.Required,
			HasProperties: tool.InputSchema.Properties != nil,
			CacheControl:  viewAnthropicV1CacheControl(&tool.CacheControl),
		})
	}
	return out
}

func viewAnthropicV1ToolChoice(tc *anthropic.ToolChoiceUnionParam) anthropicToolChoiceView {
	v := anthropicToolChoiceView{
		HasAuto: tc.OfAuto != nil,
		HasAny:  tc.OfAny != nil,
		HasTool: tc.OfTool != nil,
	}
	if tc.OfTool != nil {
		v.ToolName = tc.OfTool.Name
	}
	return v
}

func viewAnthropicV1Request(req *anthropic.MessageNewParams) anthropicRequestView {
	view := anthropicRequestView{
		Model:           string(req.Model),
		MaxTokens:       req.MaxTokens,
		CacheControl:    viewAnthropicV1CacheControl(&req.CacheControl),
		HasTools:        len(req.Tools) > 0,
		Tools:           viewAnthropicV1Tools(req.Tools),
		ToolChoice:      viewAnthropicV1ToolChoice(&req.ToolChoice),
		ThinkingEnabled: req.Thinking.OfEnabled != nil || req.Thinking.OfAdaptive != nil || IsThinkingEnabled(req),
		OutputEffort:    string(req.OutputConfig.Effort),
	}
	for _, sys := range req.System {
		view.System = append(view.System, sys)
	}
	for _, msg := range req.Messages {
		view.Messages = append(view.Messages, viewAnthropicV1Message(msg))
	}
	return view
}

// ───────────────────────── beta adapters ─────────────────────────

func viewAnthropicBetaBlock(block anthropic.BetaContentBlockParamUnion) anthropicBlockView {
	cacheControl := block.GetCacheControl()
	cache := viewAnthropicBetaCacheControl(cacheControl)
	switch {
	case block.OfText != nil:
		return anthropicBlockView{Kind: blockViewText, Text: block.OfText.Text, CacheControl: cache}
	case block.OfThinking != nil:
		return anthropicBlockView{Kind: blockViewThinking, Text: block.OfThinking.Thinking}
	case block.OfRedactedThinking != nil:
		return anthropicBlockView{Kind: blockViewRedactedThinking}
	case block.OfToolUse != nil:
		return anthropicBlockView{
			Kind:         blockViewToolUse,
			ToolID:       block.OfToolUse.ID,
			ToolName:     block.OfToolUse.Name,
			ToolInput:    block.OfToolUse.Input,
			CacheControl: cache,
		}
	case block.OfToolResult != nil:
		return anthropicBlockView{
			Kind:           blockViewToolResult,
			ToolResultID:   block.OfToolResult.ToolUseID,
			ToolResultText: convertBetaToolResultContent(block.OfToolResult.Content),
			CacheControl:   cache,
		}
	case block.OfImage != nil:
		v := anthropicBlockView{Kind: blockViewImage, CacheControl: cache}
		if block.OfImage.Source.OfBase64 != nil {
			v.ImageMediaType = string(block.OfImage.Source.OfBase64.MediaType)
			v.ImageData = block.OfImage.Source.OfBase64.Data
		} else if block.OfImage.Source.OfURL != nil {
			v.ImageURL = block.OfImage.Source.OfURL.URL
		}
		return v
	}
	return anthropicBlockView{Kind: blockViewSkip}
}

func viewAnthropicBetaMessage(msg anthropic.BetaMessageParam) anthropicMessageView {
	blocks := make([]anthropicBlockView, 0, len(msg.Content))
	for _, block := range msg.Content {
		blocks = append(blocks, viewAnthropicBetaBlock(block))
	}
	return anthropicMessageView{Role: string(msg.Role), Blocks: blocks}
}

func viewAnthropicBetaTools(tools []anthropic.BetaToolUnionParam) []anthropicToolView {
	if len(tools) == 0 {
		return nil
	}
	out := make([]anthropicToolView, 0, len(tools))
	for _, t := range tools {
		tool := t.OfTool
		if tool == nil {
			continue
		}
		out = append(out, anthropicToolView{
			Name:          tool.Name,
			Description:   tool.Description.Value,
			InputSchema:   tool.InputSchema,
			Properties:    tool.InputSchema.Properties,
			Required:      tool.InputSchema.Required,
			HasProperties: tool.InputSchema.Properties != nil,
			CacheControl:  viewAnthropicBetaCacheControl(&tool.CacheControl),
		})
	}
	return out
}

func viewAnthropicBetaToolChoice(tc *anthropic.BetaToolChoiceUnionParam) anthropicToolChoiceView {
	v := anthropicToolChoiceView{
		HasAuto: tc.OfAuto != nil,
		HasAny:  tc.OfAny != nil,
		HasTool: tc.OfTool != nil,
	}
	if tc.OfTool != nil {
		v.ToolName = tc.OfTool.Name
	}
	return v
}

func viewAnthropicBetaRequest(req *anthropic.BetaMessageNewParams) anthropicRequestView {
	view := anthropicRequestView{
		Model:           string(req.Model),
		MaxTokens:       req.MaxTokens,
		CacheControl:    viewAnthropicBetaCacheControl(&req.CacheControl),
		HasTools:        len(req.Tools) > 0,
		Tools:           viewAnthropicBetaTools(req.Tools),
		ToolChoice:      viewAnthropicBetaToolChoice(&req.ToolChoice),
		ThinkingEnabled: req.Thinking.OfEnabled != nil || req.Thinking.OfAdaptive != nil || IsThinkingEnabledBeta(req),
		OutputEffort:    string(req.OutputConfig.Effort),
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
		case "user":
			// User messages may contain tool_result blocks - need special handling
			openaiReq.Messages = append(openaiReq.Messages, convertAnthropicViewUserToOpenAI(msg.Blocks)...)
		case "assistant":
			// Convert assistant message with potential tool_use blocks
			openaiReq.Messages = append(openaiReq.Messages, convertAnthropicViewAssistantToOpenAI(msg.Blocks))
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
	if view.HasTools {
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
	if view.ThinkingEnabled {
		config.HasThinking = true
		config.ReasoningEffort = "medium"
	}
	if view.OutputEffort != "" {
		config.ReasoningEffort = shared.ReasoningEffort(view.OutputEffort)
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
func convertAnthropicViewAssistantToOpenAI(blocks []anthropicBlockView) openai.ChatCompletionMessageParamUnion {
	preserveTextParts := blocksHaveCacheControl(blocks)
	var textContent strings.Builder
	var textParts []openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion
	var toolCalls []openai.ChatCompletionMessageToolCallUnionParam
	var thinking string

	for _, block := range blocks {
		switch block.Kind {
		case blockViewText:
			if preserveTextParts {
				part := openAITextPart(block.Text, hasAnthropicCacheControl(block.CacheControl))
				textParts = append(textParts, openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion{
					OfText: &part,
				})
			} else {
				textContent.WriteString(block.Text)
			}
		case blockViewToolUse:
			// Convert tool_use block to OpenAI tool_call format;
			// marshal input to a JSON string for OpenAI
			var args string
			if argsBytes, err := json.Marshal(block.ToolInput); err == nil {
				args = string(argsBytes)
			}
			toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: block.ToolID,
					Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      block.ToolName,
						Arguments: args,
					},
				},
			})
		case blockViewThinking:
			thinking = block.Text
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
func convertAnthropicViewUserToOpenAI(blocks []anthropicBlockView) []openai.ChatCompletionMessageParamUnion {
	var result []openai.ChatCompletionMessageParamUnion
	var hasToolResult, hasImage, hasCache bool

	for _, block := range blocks {
		switch block.Kind {
		case blockViewToolResult:
			hasToolResult = true
		case blockViewImage:
			hasImage = true
		}
		hasCache = hasCache || hasAnthropicCacheControl(block.CacheControl)
	}

	switch {
	case hasToolResult:
		// When there are tool_result blocks, we need to create separate messages
		var textBlocks []anthropicBlockView
		for _, block := range blocks {
			switch block.Kind {
			case blockViewText:
				textBlocks = append(textBlocks, block)
			case blockViewToolResult:
				// Convert tool_result to OpenAI role="tool" message.
				// Truncate tool_call_id to meet OpenAI's 40 character limit.
				if hasAnthropicCacheControl(block.CacheControl) {
					part := openAITextPart(block.ToolResultText, true)
					result = append(result, openai.ChatCompletionMessageParamUnion{
						OfTool: &openai.ChatCompletionToolMessageParam{
							ToolCallID: truncateToolCallID(block.ToolResultID),
							Content: openai.ChatCompletionToolMessageParamContentUnion{
								OfArrayOfContentParts: []openai.ChatCompletionContentPartTextParam{part},
							},
						},
					})
				} else {
					result = append(result, openai.ToolMessage(block.ToolResultText, truncateToolCallID(block.ToolResultID)))
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
			switch block.Kind {
			case blockViewText:
				part := openAITextPart(block.Text, hasAnthropicCacheControl(block.CacheControl))
				parts = append(parts, openai.ChatCompletionContentPartUnionParam{OfText: &part})
			case blockViewImage:
				url := imageViewToOpenAIURL(block)
				if url == "" {
					continue
				}
				imagePart := openai.ChatCompletionContentPartImageParam{
					ImageURL: openai.ChatCompletionContentPartImageImageURLParam{URL: url},
				}
				if hasAnthropicCacheControl(block.CacheControl) {
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
			if block.Kind == blockViewText {
				textContent.WriteString(block.Text)
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

func blocksHaveCacheControl(blocks []anthropicBlockView) bool {
	for _, block := range blocks {
		if hasAnthropicCacheControl(block.CacheControl) &&
			(block.Kind == blockViewText || block.Kind == blockViewImage || block.Kind == blockViewToolResult) {
			return true
		}
	}
	return false
}

func viewHasRepresentableCacheControl(view anthropicRequestView) bool {
	if systemBlocksHaveCacheControl(view.System) {
		return true
	}
	for _, message := range view.Messages {
		if blocksHaveCacheControl(message.Blocks) {
			return true
		}
	}
	return false
}

func viewHasToolDefinitionCacheControl(view anthropicRequestView) bool {
	for _, tool := range view.Tools {
		if hasAnthropicCacheControl(tool.CacheControl) {
			return true
		}
	}
	return false
}

func viewHasToolUseCacheControl(view anthropicRequestView) bool {
	for _, message := range view.Messages {
		for _, block := range message.Blocks {
			if block.Kind == blockViewToolUse && hasAnthropicCacheControl(block.CacheControl) {
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

// imageViewToOpenAIURL renders an image block view as the URL string OpenAI's
// image_url content part expects. Base64 sources become a data: URL; URL
// sources are passed through. Returns "" for unsupported sources.
func imageViewToOpenAIURL(block anthropicBlockView) string {
	if block.ImageData != "" {
		return "data:" + block.ImageMediaType + ";base64," + block.ImageData
	}
	return block.ImageURL
}

// convertAnthropicToolViewsToOpenAI converts normalized tool definitions to
// OpenAI function tools.
func convertAnthropicToolViewsToOpenAI(tools []anthropicToolView) []openai.ChatCompletionToolUnionParam {
	// nil means the request declared no tools at all; a non-nil empty view
	// slice (tools declared, none convertible — e.g. only server tools)
	// must keep producing "tools": [] on the wire, exactly as before.
	if tools == nil {
		return nil
	}

	out := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
	for _, tool := range tools {
		// Convert Anthropic input schema to OpenAI function parameters
		parameters := convertAnthropicInputSchemaToOpenAIParameters(tool.Properties, tool.Required)

		out = append(out, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        tool.Name,
			Description: param.Opt[string]{Value: tool.Description},
			Parameters:  parameters,
		}))
	}
	return out
}

// convertAnthropicToolChoiceViewToOpenAI converts a normalized tool_choice to
// OpenAI format. Anthropic's "any" (required) maps to auto, as OpenAI has no
// direct equivalent.
func convertAnthropicToolChoiceViewToOpenAI(tc anthropicToolChoiceView) openai.ChatCompletionToolChoiceOptionUnionParam {
	if tc.HasTool {
		return openai.ToolChoiceOptionFunctionToolChoice(
			openai.ChatCompletionNamedToolChoiceFunctionParam{
				Name: tc.ToolName,
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
	if view.HasTools {
		config.Tools = []*genai.Tool{
			{
				FunctionDeclarations: convertAnthropicToolViewsToGoogle(view.Tools),
			},
		}
	}

	// Convert tool choice
	if view.ToolChoice.HasAuto || view.ToolChoice.HasTool || view.ToolChoice.HasAny {
		config.ToolConfig = convertAnthropicToolChoiceViewToGoogle(view.ToolChoice)
	}

	return view.Model, contents, config
}

// convertAnthropicViewMessageToGoogle converts one normalized message to a
// Google content. Returns nil when the message produces no parts or has an
// unsupported role.
func convertAnthropicViewMessageToGoogle(msg anthropicMessageView) *genai.Content {
	switch msg.Role {
	case "user":
		content := &genai.Content{
			Role:  "user",
			Parts: []*genai.Part{},
		}
		for _, block := range msg.Blocks {
			switch block.Kind {
			case blockViewText:
				content.Parts = append(content.Parts, genai.NewPartFromText(block.Text))
			case blockViewImage:
				// For Google API, images need to be passed as inline data with MIME type
				if block.ImageData != "" {
					content.Parts = append(content.Parts, &genai.Part{
						InlineData: &genai.Blob{
							MIMEType: block.ImageMediaType,
							Data:     []byte(block.ImageData),
						},
					})
				} else if block.ImageURL != "" {
					// For URL images, we'd need to fetch them first
					// For now, skip or handle as text reference
					content.Parts = append(content.Parts, genai.NewPartFromText("[Image: "+block.ImageURL+"]"))
				}
			case blockViewToolResult:
				// Convert tool_result to function_response.
				// FunctionResponse.Name should be the tool_use ID for Google API.
				// Try to parse as JSON first; if it fails, wrap as plain text output.
				var response map[string]any
				if err := json.Unmarshal([]byte(block.ToolResultText), &response); err != nil {
					// Not valid JSON, wrap in "output" key
					response = map[string]any{"output": block.ToolResultText}
				}
				content.Parts = append(content.Parts, &genai.Part{
					FunctionResponse: &genai.FunctionResponse{
						Name:     block.ToolResultID, // Use tool_use ID as Name
						Response: response,
					},
				})
			case blockViewThinking, blockViewRedactedThinking:
				// Skip thinking blocks - Google API doesn't support them
			}
		}
		if len(content.Parts) == 0 {
			return nil
		}
		return content

	case "assistant":
		content := &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{},
		}
		for _, block := range msg.Blocks {
			switch block.Kind {
			case blockViewText:
				content.Parts = append(content.Parts, genai.NewPartFromText(block.Text))
			case blockViewToolUse:
				// Convert tool_use to function_call
				var argsInput map[string]interface{}
				if inputBytes, ok := block.ToolInput.([]byte); ok {
					_ = json.Unmarshal(inputBytes, &argsInput)
				}
				content.Parts = append(content.Parts, &genai.Part{
					FunctionCall: &genai.FunctionCall{
						ID:   block.ToolID,
						Name: block.ToolName,
						Args: argsInput,
					},
				})
			case blockViewThinking, blockViewRedactedThinking:
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
func convertAnthropicToolViewsToGoogle(tools []anthropicToolView) []*genai.FunctionDeclaration {
	// nil means no tools declared; non-nil empty keeps the previous
	// empty-declarations shape for requests whose tools were all filtered.
	if tools == nil {
		return nil
	}

	out := make([]*genai.FunctionDeclaration, 0, len(tools))
	for _, tool := range tools {
		// Convert Anthropic input schema to Google parameters
		var parameters *genai.Schema
		if tool.HasProperties {
			if schemaBytes, err := json.Marshal(tool.InputSchema); err == nil {
				_ = json.Unmarshal(schemaBytes, &parameters)
				// Normalize schema types from lowercase (JSON Schema) to uppercase (Google format)
				NormalizeSchemaTypes(parameters)
			}
		}

		out = append(out, &genai.FunctionDeclaration{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  parameters,
		})
	}
	return out
}

// convertAnthropicToolChoiceViewToGoogle converts a normalized tool_choice to
// a Google tool config.
func convertAnthropicToolChoiceViewToGoogle(tc anthropicToolChoiceView) *genai.ToolConfig {
	config := &genai.ToolConfig{
		FunctionCallingConfig: &genai.FunctionCallingConfig{},
	}

	if tc.HasAuto {
		config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAuto
	}
	if tc.HasTool {
		config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAny
		config.FunctionCallingConfig.AllowedFunctionNames = []string{tc.ToolName}
	}
	if tc.HasAny {
		config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAny
	}

	// Default to auto
	if config.FunctionCallingConfig.Mode == "" {
		config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAuto
	}
	return config
}
