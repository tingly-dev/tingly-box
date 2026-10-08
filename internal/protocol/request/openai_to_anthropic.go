package request

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	openaiparam "github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"

	"github.com/tingly-dev/tingly-box/internal/protocol/thinking"
)

// ConvertOpenAIToAnthropicRequest converts OpenAI ChatCompletionNewParams to Anthropic SDK format
func ConvertOpenAIToAnthropicRequest(req *openai.ChatCompletionNewParams, defaultMaxTokens int64) *anthropic.BetaMessageNewParams {
	messages := make([]anthropic.BetaMessageParam, 0, len(req.Messages))
	var systemBlocks []anthropic.BetaTextBlockParam

	for _, msg := range req.Messages {
		// Read the typed union fields directly — no JSON round-trip needed.
		switch {
		case msg.OfSystem != nil:
			// System message → params.System (string or array-of-text form)
			if content := msg.OfSystem.Content.OfString.Value; content != "" {
				systemBlocks = append(systemBlocks, anthropic.BetaTextBlockParam{Text: content})
			} else {
				for _, part := range msg.OfSystem.Content.OfArrayOfContentParts {
					if part.Text == "" {
						continue
					}
					systemBlocks = append(systemBlocks, betaTextBlockFromOpenAI(part))
				}
			}

		case msg.OfUser != nil:
			// User message
			var blocks []anthropic.BetaContentBlockParamUnion

			if content := msg.OfUser.Content.OfString.Value; content != "" {
				// Simple text content
				blocks = append(blocks, anthropic.NewBetaTextBlock(content))
			} else {
				// Array of content parts (multimodal)
				for _, part := range msg.OfUser.Content.OfArrayOfContentParts {
					switch {
					case part.OfText != nil:
						if part.OfText.Text != "" {
							block := anthropic.NewBetaTextBlock(part.OfText.Text)
							if hasOpenAITextCacheBreakpoint(*part.OfText) {
								block.OfText.CacheControl = anthropic.NewBetaCacheControlEphemeralParam()
							}
							blocks = append(blocks, block)
						}
					case part.OfImageURL != nil:
						if block, ok := openAIImageURLToAnthropicBetaBlock(part.OfImageURL.ImageURL.URL); ok {
							if !openaiparam.IsOmitted(part.OfImageURL.PromptCacheBreakpoint) {
								block.OfImage.CacheControl = anthropic.NewBetaCacheControlEphemeralParam()
							}
							blocks = append(blocks, block)
						}
					}
				}
			}

			if len(blocks) > 0 {
				messages = append(messages, anthropic.NewBetaUserMessage(blocks...))
			}

		case msg.OfAssistant != nil:
			// Assistant message
			var blocks []anthropic.BetaContentBlockParamUnion

			// Add text content if present (string or array-of-text form)
			if content := msg.OfAssistant.Content.OfString.Value; content != "" {
				blocks = append(blocks, anthropic.NewBetaTextBlock(content))
			} else {
				for _, part := range msg.OfAssistant.Content.OfArrayOfContentParts {
					if part.OfText == nil || part.OfText.Text == "" {
						continue
					}
					block := anthropic.NewBetaTextBlock(part.OfText.Text)
					if hasOpenAITextCacheBreakpoint(*part.OfText) {
						block.OfText.CacheControl = anthropic.NewBetaCacheControlEphemeralParam()
					}
					blocks = append(blocks, block)
				}
			}

			// Convert tool calls to tool_use blocks
			for _, tc := range msg.OfAssistant.ToolCalls {
				fn := tc.OfFunction
				if fn == nil {
					continue
				}
				var argsInput interface{}
				if fn.Function.Arguments != "" {
					_ = json.Unmarshal([]byte(fn.Function.Arguments), &argsInput)
				}
				blocks = append(blocks,
					anthropic.NewBetaToolUseBlock(fn.ID, argsInput, fn.Function.Name),
				)
			}

			if len(blocks) > 0 {
				messages = append(messages, anthropic.BetaMessageParam{
					Content: blocks,
					Role:    anthropic.BetaMessageParamRoleAssistant,
				})
			}

		case msg.OfTool != nil:
			// Tool result message → tool_result block (must be USER role).
			// Content may be a plain string or an array of content parts;
			// image_url parts (tool screenshots — issue #1606) become image
			// blocks inside the tool_result content.
			var block anthropic.BetaContentBlockParamUnion
			hasCacheControl := false
			if content := msg.OfTool.Content.OfString.Value; content != "" {
				block = anthropic.NewBetaToolResultBlock(msg.OfTool.ToolCallID, content, false)
			} else {
				var resultBlocks []anthropic.BetaToolResultBlockParamContentUnion
				for _, part := range msg.OfTool.Content.OfArrayOfContentParts {
					switch {
					case part.OfText != nil:
						if part.OfText.Text == "" {
							continue
						}
						resultBlocks = append(resultBlocks, anthropic.BetaToolResultBlockParamContentUnion{
							OfText: &anthropic.BetaTextBlockParam{Text: part.OfText.Text},
						})
						hasCacheControl = hasCacheControl || hasOpenAITextCacheBreakpoint(*part.OfText)
					case part.OfImageURL != nil:
						imageBlock, ok := openAIImageURLToAnthropicBetaBlock(part.OfImageURL.ImageURL.URL)
						if !ok {
							continue
						}
						resultBlocks = append(resultBlocks, anthropic.BetaToolResultBlockParamContentUnion{
							OfImage: imageBlock.OfImage,
						})
						hasCacheControl = hasCacheControl || !openaiparam.IsOmitted(part.OfImageURL.PromptCacheBreakpoint)
					}
				}
				block = anthropic.BetaContentBlockParamUnion{OfToolResult: &anthropic.BetaToolResultBlockParam{
					ToolUseID: msg.OfTool.ToolCallID,
					Content:   resultBlocks,
				}}
			}
			if hasCacheControl {
				block.OfToolResult.CacheControl = anthropic.NewBetaCacheControlEphemeralParam()
			}
			blocks := []anthropic.BetaContentBlockParamUnion{block}
			messages = append(messages, anthropic.NewBetaUserMessage(blocks...))
		}
	}

	// Determine max_tokens: the modern max_completion_tokens first, then the
	// deprecated max_tokens, else the default.
	maxTokens := req.MaxCompletionTokens.Value
	if maxTokens == 0 {
		maxTokens = req.MaxTokens.Value
	}
	limitSet := maxTokens != 0
	if !limitSet {
		maxTokens = defaultMaxTokens
	}

	params := &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(req.Model),
		Messages:  messages,
		MaxTokens: maxTokens,
	}
	if req.PromptCacheOptions.Mode == "implicit" {
		params.CacheControl = anthropic.NewBetaCacheControlEphemeralParam()
	}
	applyOpenAIEffortAsThinking(params, req.ReasoningEffort, limitSet)

	// Add system blocks if any. Array-form OpenAI content keeps standard
	// prompt_cache_breakpoint markers from an earlier Anthropic hop.
	if len(systemBlocks) > 0 {
		params.System = systemBlocks
	}

	// Convert tools from OpenAI format to Anthropic format
	if len(req.Tools) > 0 {
		params.Tools = ConvertOpenAIToAnthropicTools(req.Tools)
		// Convert tool choice
		// ToolChoice is a Union type, check if any field is set
		params.ToolChoice = ConvertOpenAIToAnthropicToolChoice(&req.ToolChoice)
	}

	reconcileGatewayThinking(params)
	return params
}

func hasOpenAITextCacheBreakpoint(part openai.ChatCompletionContentPartTextParam) bool {
	return !openaiparam.IsOmitted(part.PromptCacheBreakpoint)
}

func betaTextBlockFromOpenAI(part openai.ChatCompletionContentPartTextParam) anthropic.BetaTextBlockParam {
	block := anthropic.BetaTextBlockParam{Text: part.Text}
	if hasOpenAITextCacheBreakpoint(part) {
		block.CacheControl = anthropic.NewBetaCacheControlEphemeralParam()
	}
	return block
}

// openAIImageURLToAnthropicBetaBlock turns an OpenAI image_url.url string into
// an Anthropic beta image content block. Data URLs become base64 image sources,
// remote URLs become URL image sources. Returns ok=false for empty/malformed
// inputs the caller should drop.
func openAIImageURLToAnthropicBetaBlock(url string) (anthropic.BetaContentBlockParamUnion, bool) {
	mediaType, data, remoteURL := ParseImageURLToAnthropicSource(url)
	switch {
	case mediaType != "" && data != "":
		return anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
			Data:      data,
			MediaType: anthropic.BetaBase64ImageSourceMediaType(mediaType),
		}), true
	case remoteURL != "":
		return anthropic.NewBetaImageBlock(anthropic.BetaURLImageSourceParam{
			URL: remoteURL,
		}), true
	}
	return anthropic.BetaContentBlockParamUnion{}, false
}

func ConvertOpenAIToAnthropicTools(tools []openai.ChatCompletionToolUnionParam) []anthropic.BetaToolUnionParam {

	if len(tools) == 0 {
		return nil
	}

	out := make([]anthropic.BetaToolUnionParam, 0, len(tools))

	for _, t := range tools {
		fn := t.GetFunction()
		if fn == nil || fn.Parameters == nil {
			continue
		}

		// Convert OpenAI function schema to Anthropic input schema with a
		// single marshal/unmarshal pass.
		schemaBytes, err := json.Marshal(fn.Parameters)
		if err != nil {
			continue
		}
		var schemaParam anthropic.BetaToolInputSchemaParam
		if err := json.Unmarshal(schemaBytes, &schemaParam); err != nil {
			continue
		}
		tool := anthropic.BetaToolUnionParam{
			OfTool: &anthropic.BetaToolParam{
				Name:        fn.Name,
				InputSchema: schemaParam,
			},
		}
		if fn.Description.Value != "" {
			tool.OfTool.Description = anthropic.Opt(fn.Description.Value)
		}
		out = append(out, tool)
	}

	return out
}

func ConvertOpenAIToAnthropicToolChoice(tc *openai.ChatCompletionToolChoiceOptionUnionParam) anthropic.BetaToolChoiceUnionParam {

	// Check the different variants
	if auto := tc.OfAuto.Value; auto != "" {
		if auto == "auto" {
			return anthropic.BetaToolChoiceUnionParam{
				OfAuto: &anthropic.BetaToolChoiceAutoParam{},
			}
		}
	}

	if tc.OfAllowedTools != nil {
		// Default to auto for allowed tools
		return anthropic.BetaToolChoiceUnionParam{
			OfAuto: &anthropic.BetaToolChoiceAutoParam{},
		}
	}

	if funcChoice := tc.OfFunctionToolChoice; funcChoice != nil {
		if name := funcChoice.Function.Name; name != "" {
			return anthropic.BetaToolChoiceParamOfTool(name)
		}
	}

	if tc.OfCustomToolChoice != nil {
		// Default to auto for custom tool choice
		return anthropic.BetaToolChoiceUnionParam{
			OfAuto: &anthropic.BetaToolChoiceAutoParam{},
		}
	}

	// Default to auto
	return anthropic.BetaToolChoiceUnionParam{
		OfAuto: &anthropic.BetaToolChoiceAutoParam{},
	}
}

// applyOpenAIEffortAsThinking carries an OpenAI client's reasoning effort to an
// Anthropic request: thinking enabled at the ladder's budget for that level
// (thinking.BudgetMapping) plus output_config.effort. "", "none" and unknown
// values leave thinking unset.
//
// Anthropic counts thinking inside max_tokens, so the budget must leave room
// for the answer:
//   - the client set no output limit (limitSet false): max_tokens, the
//     default answer room, grows by the budget;
//   - the client set one: the client's limit is the whole output, so the
//     budget takes at most half of it, and a limit too small for the 1024
//     minimum leaves thinking off.
//
// The target half then caps both at the model limit (output_limit) and, on a
// Claude Code backend, reconciles thinking with the model's dialects.
//
// Once the conversion is complete, reconcileGatewayThinking makes this
// thinking legal with the rest of the request.
func applyOpenAIEffortAsThinking(params *anthropic.BetaMessageNewParams, effort shared.ReasoningEffort, limitSet bool) {
	level := string(effort)
	budget, ok := thinking.BudgetMapping[level]
	if !ok {
		return
	}
	if limitSet {
		if params.MaxTokens <= minThinkingBudget {
			return
		}
		budget = min(budget, max(minThinkingBudget, params.MaxTokens/2))
	} else {
		params.MaxTokens += budget
	}
	params.Thinking = anthropic.BetaThinkingConfigParamOfEnabled(budget)
	params.OutputConfig.Effort = anthropic.BetaOutputConfigEffort(thinking.AnthropicEffort(level))
}

// minThinkingBudget is Anthropic's smallest accepted budget_tokens.
const minThinkingBudget int64 = 1024

// reconcileGatewayThinking runs once a conversion has built the whole
// request: the thinking it produced from an OpenAI client's effort sits on a
// history that carries no thinking blocks, so a tool-use turn needs thinking
// turned off, a forced tool_choice too, and the sampling parameters thinking
// forbids are dropped (thinking.ReconcileBetaWithRequest). It runs here,
// where that thinking is produced, so the vendor stage need not tell
// gateway-produced thinking from a client's own: a client's own request
// reaches a third-party provider as it came.
func reconcileGatewayThinking(params *anthropic.BetaMessageNewParams) {
	if params.Thinking.OfEnabled != nil {
		thinking.ReconcileBetaWithRequest(params)
	}
}
