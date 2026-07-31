package request

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicparam "github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/openai/openai-go/v3"
	openaiparam "github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/require"
)

func TestAnthropicOpenAIAnthropicPreservesCacheControl(t *testing.T) {
	userBlock := anthropic.NewTextBlock("stable conversation prefix")
	userBlock.OfText.CacheControl = anthropic.NewCacheControlEphemeralParam()

	in := &anthropic.MessageNewParams{
		Model:     "claude-test",
		MaxTokens: 256,
		System: []anthropic.TextBlockParam{
			{
				Text:         "stable system prompt",
				CacheControl: anthropic.NewCacheControlEphemeralParam(),
			},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(userBlock),
		},
	}

	openAIReq, _ := ConvertAnthropicToOpenAIRequest(in, true, true, false)
	require.Equal(t, "explicit", openAIReq.PromptCacheOptions.Mode)
	require.Len(t, openAIReq.Messages, 2)

	systemParts := openAIReq.Messages[0].OfSystem.Content.OfArrayOfContentParts
	require.Len(t, systemParts, 1)
	require.False(t, openaiparam.IsOmitted(systemParts[0].PromptCacheBreakpoint))

	userParts := openAIReq.Messages[1].OfUser.Content.OfArrayOfContentParts
	require.Len(t, userParts, 1)
	require.NotNil(t, userParts[0].OfText)
	require.False(t, openaiparam.IsOmitted(userParts[0].OfText.PromptCacheBreakpoint))

	wire, err := json.Marshal(openAIReq)
	require.NoError(t, err)
	require.Contains(t, string(wire), `"prompt_cache_breakpoint":{"mode":"explicit"}`)

	out := ConvertOpenAIToAnthropicRequest(openAIReq, 4096)
	require.Len(t, out.System, 1)
	require.False(t, anthropicparam.IsOmitted(out.System[0].CacheControl))
	require.Len(t, out.Messages, 1)
	require.Len(t, out.Messages[0].Content, 1)
	require.NotNil(t, out.Messages[0].Content[0].OfText)
	require.False(t, anthropicparam.IsOmitted(out.Messages[0].Content[0].OfText.CacheControl))
}

func TestAnthropicAutomaticCacheControlRoundTripsThroughOpenAIWire(t *testing.T) {
	t.Run("chat", func(t *testing.T) {
		in := &anthropic.MessageNewParams{
			Model:        "claude-test",
			MaxTokens:    256,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
			System:       []anthropic.TextBlockParam{{Text: "stable system prompt"}},
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("growing conversation")),
			},
		}

		chat, _ := ConvertAnthropicToOpenAIRequest(in, true, false, false)
		require.Equal(t, "implicit", chat.PromptCacheOptions.Mode)

		wire, err := json.Marshal(chat)
		require.NoError(t, err)
		require.Contains(t, string(wire), `"prompt_cache_options":{"mode":"implicit"}`)

		var reparsed openai.ChatCompletionNewParams
		require.NoError(t, json.Unmarshal(wire, &reparsed))
		out := ConvertOpenAIToAnthropicRequest(&reparsed, 4096)
		require.False(t, anthropicparam.IsOmitted(out.CacheControl))
	})

	t.Run("responses", func(t *testing.T) {
		in := &anthropic.MessageNewParams{
			Model:        "claude-test",
			MaxTokens:    256,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
			System:       []anthropic.TextBlockParam{{Text: "stable system prompt"}},
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("growing conversation")),
			},
		}

		responsesReq := ConvertAnthropicV1ToResponsesRequest(in)
		require.Equal(t, "implicit", responsesReq.PromptCacheOptions.Mode)

		wire, err := json.Marshal(responsesReq)
		require.NoError(t, err)
		require.Contains(t, string(wire), `"prompt_cache_options":{"mode":"implicit"}`)

		var reparsed responses.ResponseNewParams
		require.NoError(t, json.Unmarshal(wire, &reparsed))
		out := ConvertOpenAIResponsesToAnthropicBetaRequest(reparsed, 4096)
		require.False(t, anthropicparam.IsOmitted(out.CacheControl))
	})
}

func TestAnthropicAutomaticAndExplicitCacheControlsCoexist(t *testing.T) {
	user := anthropic.NewTextBlock("stable conversation prefix")
	user.OfText.CacheControl = anthropic.NewCacheControlEphemeralParam()
	in := &anthropic.MessageNewParams{
		Model:        "claude-test",
		MaxTokens:    256,
		CacheControl: anthropic.NewCacheControlEphemeralParam(),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(user),
		},
	}
	chat, _ := ConvertAnthropicToOpenAIRequest(in, true, false, false)
	require.Equal(t, "implicit", chat.PromptCacheOptions.Mode)
	require.False(t, openaiparam.IsOmitted(
		chat.Messages[0].OfUser.Content.OfArrayOfContentParts[0].OfText.PromptCacheBreakpoint))

	responsesReq := ConvertAnthropicV1ToResponsesRequest(in)
	require.Equal(t, "implicit", responsesReq.PromptCacheOptions.Mode)
	requireResponsesTextBreakpoint(t, responsesReq.Input.OfInputItemList[0], "user")
}

func TestAnthropicCacheControlTTLDegradesExplicitlyAcrossOpenAI(t *testing.T) {
	in := &anthropic.MessageNewParams{
		Model:     "claude-test",
		MaxTokens: 256,
		CacheControl: anthropic.CacheControlEphemeralParam{
			Type: "ephemeral",
			TTL:  anthropic.CacheControlEphemeralTTLTTL1h,
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("growing conversation")),
		},
	}
	view := viewAnthropicV1Request(in)
	require.False(t, anthropicparam.IsOmitted(view.CacheControl))
	require.Equal(t, anthropic.CacheControlEphemeralTTLTTL1h, view.CacheControl.TTL)

	chat, _ := ConvertAnthropicToOpenAIRequest(in, true, false, false)
	require.Equal(t, "implicit", chat.PromptCacheOptions.Mode)
	require.Empty(t, chat.PromptCacheOptions.Ttl,
		"Anthropic 1h must not be mistranslated as OpenAI's only supported 30m TTL")

	out := ConvertOpenAIToAnthropicRequest(chat, 4096)
	require.False(t, anthropicparam.IsOmitted(out.CacheControl))
	require.Empty(t, out.CacheControl.TTL,
		"cross-family round trip preserves automatic caching but Anthropic 1h has no OpenAI equivalent")
}

func TestAnthropicViewPreservesSDKCacheControlShape(t *testing.T) {
	cache1h := anthropic.CacheControlEphemeralParam{
		Type: "ephemeral",
		TTL:  anthropic.CacheControlEphemeralTTLTTL1h,
	}
	cache5m := anthropic.CacheControlEphemeralParam{
		Type: "ephemeral",
		TTL:  anthropic.CacheControlEphemeralTTLTTL5m,
	}
	user := anthropic.NewTextBlock("stable conversation prefix")
	user.OfText.CacheControl = cache1h
	in := &anthropic.MessageNewParams{
		Model:        "claude-test",
		MaxTokens:    256,
		CacheControl: cache1h,
		System: []anthropic.TextBlockParam{{
			Text:         "stable system prompt",
			CacheControl: cache5m,
		}},
		Messages: []anthropic.MessageParam{anthropic.NewUserMessage(user)},
		Tools: []anthropic.ToolUnionParam{{
			OfTool: &anthropic.ToolParam{
				Name:         "lookup",
				InputSchema:  anthropic.ToolInputSchemaParam{Type: "object"},
				CacheControl: cache1h,
			},
		}},
	}

	view := viewAnthropicV1Request(in)
	require.Equal(t, cache1h.TTL, view.CacheControl.TTL)
	require.Equal(t, cache5m.TTL, view.System[0].CacheControl.TTL)
	require.Equal(t, cache1h.TTL, view.Messages[0].Content[0].OfText.CacheControl.TTL)
	require.Equal(t, cache1h.TTL, view.Tools[0].OfTool.CacheControl.TTL)

	betaCache1h := anthropic.BetaCacheControlEphemeralParam{
		Type: "ephemeral",
		TTL:  anthropic.BetaCacheControlEphemeralTTLTTL1h,
	}
	betaCache5m := anthropic.BetaCacheControlEphemeralParam{
		Type: "ephemeral",
		TTL:  anthropic.BetaCacheControlEphemeralTTLTTL5m,
	}
	betaUser := anthropic.NewBetaTextBlock("stable conversation prefix")
	betaUser.OfText.CacheControl = betaCache1h
	betaIn := &anthropic.BetaMessageNewParams{
		Model:        "claude-test",
		MaxTokens:    256,
		CacheControl: betaCache1h,
		System: []anthropic.BetaTextBlockParam{{
			Text:         "stable system prompt",
			CacheControl: betaCache5m,
		}},
		Messages: []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(betaUser)},
		Tools: []anthropic.BetaToolUnionParam{{
			OfTool: &anthropic.BetaToolParam{
				Name:         "lookup",
				InputSchema:  anthropic.BetaToolInputSchemaParam{Type: "object"},
				CacheControl: betaCache1h,
			},
		}},
	}

	betaView := viewAnthropicBetaRequest(betaIn)
	require.Equal(t, anthropic.CacheControlEphemeralTTLTTL1h, betaView.CacheControl.TTL)
	require.Equal(t, anthropic.CacheControlEphemeralTTLTTL5m, betaView.System[0].CacheControl.TTL)
	require.Equal(t, anthropic.CacheControlEphemeralTTLTTL1h, betaView.Messages[0].Content[0].OfText.CacheControl.TTL)
	require.Equal(t, anthropic.CacheControlEphemeralTTLTTL1h, betaView.Tools[0].OfTool.CacheControl.TTL)
}

func TestAnthropicViewUsesSDKCanonicalShape(t *testing.T) {
	t.Run("v1 fields pass through without flattening", func(t *testing.T) {
		text := anthropic.NewTextBlock("hello")
		tool := anthropic.ToolUnionParamOfTool(
			anthropic.ToolInputSchemaParam{Type: "object", Properties: map[string]any{"q": map[string]any{"type": "string"}}},
			"lookup",
		)
		toolChoice := anthropic.ToolChoiceParamOfTool("lookup")
		thinking := anthropic.ThinkingConfigParamOfEnabled(2048)
		in := &anthropic.MessageNewParams{
			Model:        "claude-test",
			MaxTokens:    256,
			Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(text)},
			Tools:        []anthropic.ToolUnionParam{tool},
			ToolChoice:   toolChoice,
			Thinking:     thinking,
			OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortHigh},
		}

		view := viewAnthropicV1Request(in)
		require.Same(t, in.Messages[0].Content[0].OfText, view.Messages[0].Content[0].OfText)
		require.Same(t, in.Tools[0].OfTool, view.Tools[0].OfTool)
		require.Same(t, in.ToolChoice.OfTool, view.ToolChoice.OfTool)
		require.Same(t, in.Thinking.OfEnabled, view.Thinking.OfEnabled)
		require.Equal(t, in.OutputConfig.Effort, view.OutputConfig.Effort)
	})

	t.Run("beta fields bridge into v1 SDK unions", func(t *testing.T) {
		image := anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
			Data:      "aGVsbG8=",
			MediaType: anthropic.BetaBase64ImageSourceMediaTypeImagePNG,
		})
		thinkingBlock := anthropic.NewBetaThinkingBlock("signature", "reasoning")
		tool := anthropic.BetaToolUnionParamOfTool(
			anthropic.BetaToolInputSchemaParam{
				Type:       "object",
				Properties: map[string]any{"q": map[string]any{"type": "string"}},
				Required:   []string{"q"},
			},
			"lookup",
		)
		toolChoice := anthropic.BetaToolChoiceParamOfTool("lookup")
		toolChoice.OfTool.DisableParallelToolUse = anthropic.Bool(true)
		thinking := anthropic.BetaThinkingConfigParamOfEnabled(2048)
		thinking.OfEnabled.Display = anthropic.BetaThinkingConfigEnabledDisplayOmitted
		in := &anthropic.BetaMessageNewParams{
			Model:     "claude-test",
			MaxTokens: 256,
			Messages: []anthropic.BetaMessageParam{{
				Role:    anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{image, thinkingBlock},
			}},
			Tools:        []anthropic.BetaToolUnionParam{tool},
			ToolChoice:   toolChoice,
			Thinking:     thinking,
			OutputConfig: anthropic.BetaOutputConfigParam{Effort: anthropic.BetaOutputConfigEffortHigh},
		}

		view := viewAnthropicBetaRequest(in)
		require.NotNil(t, view.Messages[0].Content[0].OfImage)
		require.Equal(t, "aGVsbG8=", view.Messages[0].Content[0].OfImage.Source.OfBase64.Data)
		require.Equal(t, "signature", view.Messages[0].Content[1].OfThinking.Signature)
		require.NotNil(t, view.Tools[0].OfTool)
		require.Equal(t, []string{"q"}, view.Tools[0].OfTool.InputSchema.Required)
		require.Equal(t, "lookup", view.ToolChoice.OfTool.Name)
		require.True(t, view.ToolChoice.OfTool.DisableParallelToolUse.Value)
		require.EqualValues(t, 2048, view.Thinking.OfEnabled.BudgetTokens)
		require.Equal(t, anthropic.ThinkingConfigEnabledDisplayOmitted, view.Thinking.OfEnabled.Display)
		require.Equal(t, anthropic.OutputConfigEffortHigh, view.OutputConfig.Effort)
	})
}

func TestAnthropicToolCacheControlAdvancesToOpenAICacheableContent(t *testing.T) {
	in := &anthropic.MessageNewParams{
		Model:     "claude-test",
		MaxTokens: 256,
		Tools: []anthropic.ToolUnionParam{{
			OfTool: &anthropic.ToolParam{
				Name: "lookup",
				InputSchema: anthropic.ToolInputSchemaParam{
					Type:       "object",
					Properties: map[string]any{},
				},
				CacheControl: anthropic.NewCacheControlEphemeralParam(),
			},
		}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("look this up")),
		},
	}

	openAIReq, _ := ConvertAnthropicToOpenAIRequest(in, true, true, false)
	require.Equal(t, "explicit", openAIReq.PromptCacheOptions.Mode)
	require.Len(t, openAIReq.Messages, 1)

	// OpenAI does not support breakpoints on tool definitions, so the boundary
	// advances to the first content block after the tools prefix.
	userParts := openAIReq.Messages[0].OfUser.Content.OfArrayOfContentParts
	require.Len(t, userParts, 1)
	require.NotNil(t, userParts[0].OfText)
	require.False(t, openaiparam.IsOmitted(userParts[0].OfText.PromptCacheBreakpoint))

	out := ConvertOpenAIToAnthropicRequest(openAIReq, 4096)
	require.Len(t, out.Messages, 1)
	require.Len(t, out.Messages[0].Content, 1)
	require.False(t, anthropicparam.IsOmitted(out.Messages[0].Content[0].OfText.CacheControl))
}

func TestAnthropicResponsesAnthropicPreservesCacheControl(t *testing.T) {
	userBlock := anthropic.NewTextBlock("stable conversation prefix")
	userBlock.OfText.CacheControl = anthropic.NewCacheControlEphemeralParam()
	in := &anthropic.MessageNewParams{
		Model:     "claude-test",
		MaxTokens: 256,
		System: []anthropic.TextBlockParam{{
			Text:         "stable system prompt",
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Messages: []anthropic.MessageParam{anthropic.NewUserMessage(userBlock)},
	}

	responsesReq := ConvertAnthropicV1ToResponsesRequest(in)
	require.Equal(t, "explicit", responsesReq.PromptCacheOptions.Mode)
	require.False(t, responsesReq.Instructions.Valid())
	require.Len(t, responsesReq.Input.OfInputItemList, 2)
	requireResponsesTextBreakpoint(t, responsesReq.Input.OfInputItemList[0], "system")
	requireResponsesTextBreakpoint(t, responsesReq.Input.OfInputItemList[1], "user")

	out := ConvertOpenAIResponsesToAnthropicBetaRequest(*responsesReq, 4096)
	require.Len(t, out.System, 1)
	require.False(t, anthropicparam.IsOmitted(out.System[0].CacheControl))
	require.Len(t, out.Messages, 1)
	require.False(t, anthropicparam.IsOmitted(out.Messages[0].Content[0].OfText.CacheControl))

	betaIn := ConvertAnthropicV1ToBetaRequest(in)
	require.NotNil(t, betaIn)
	betaResponsesReq := ConvertAnthropicBetaToResponsesRequest(betaIn)
	require.Equal(t, "explicit", betaResponsesReq.PromptCacheOptions.Mode)
	require.Len(t, betaResponsesReq.Input.OfInputItemList, 2)
	requireResponsesTextBreakpoint(t, betaResponsesReq.Input.OfInputItemList[0], "system")
	requireResponsesTextBreakpoint(t, betaResponsesReq.Input.OfInputItemList[1], "user")
}

func TestAnthropicResponsesPreservesToolCacheControls(t *testing.T) {
	t.Run("tool definition advances to first content", func(t *testing.T) {
		in := &anthropic.MessageNewParams{
			Model:     "claude-test",
			MaxTokens: 256,
			Tools: []anthropic.ToolUnionParam{{
				OfTool: &anthropic.ToolParam{
					Name: "lookup",
					InputSchema: anthropic.ToolInputSchemaParam{
						Type:       "object",
						Properties: map[string]any{},
					},
					CacheControl: anthropic.NewCacheControlEphemeralParam(),
				},
			}},
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("look this up")),
			},
		}

		responsesReq := ConvertAnthropicV1ToResponsesRequest(in)
		require.Equal(t, "explicit", responsesReq.PromptCacheOptions.Mode)
		require.Len(t, responsesReq.Input.OfInputItemList, 1)
		requireResponsesTextBreakpoint(t, responsesReq.Input.OfInputItemList[0], "user")
	})

	t.Run("tool result round trip", func(t *testing.T) {
		toolResult := anthropic.NewToolResultBlock("call_1", "stable tool output", false)
		toolResult.OfToolResult.CacheControl = anthropic.NewCacheControlEphemeralParam()
		in := &anthropic.MessageNewParams{
			Model:     "claude-test",
			MaxTokens: 256,
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(toolResult),
			},
		}

		responsesReq := ConvertAnthropicV1ToResponsesRequest(in)
		require.Equal(t, "explicit", responsesReq.PromptCacheOptions.Mode)
		require.Len(t, responsesReq.Input.OfInputItemList, 1)
		output := responsesReq.Input.OfInputItemList[0].OfFunctionCallOutput
		require.NotNil(t, output)
		require.Len(t, output.Output.OfResponseFunctionCallOutputItemArray, 1)
		require.False(t, openaiparam.IsOmitted(
			output.Output.OfResponseFunctionCallOutputItemArray[0].OfInputText.PromptCacheBreakpoint))

		out := ConvertOpenAIResponsesToAnthropicBetaRequest(*responsesReq, 4096)
		require.Len(t, out.Messages, 1)
		require.NotNil(t, out.Messages[0].Content[0].OfToolResult)
		require.False(t, anthropicparam.IsOmitted(out.Messages[0].Content[0].OfToolResult.CacheControl))
	})
}

func TestChatResponsesChatPreservesCacheControlsAndOptions(t *testing.T) {
	systemPart := openai.ChatCompletionContentPartTextParam{
		Text:                  "stable system prompt",
		PromptCacheBreakpoint: openai.NewChatCompletionContentPartTextPromptCacheBreakpointParam(),
	}
	userPart := openai.ChatCompletionContentPartTextParam{
		Text:                  "stable conversation prefix",
		PromptCacheBreakpoint: openai.NewChatCompletionContentPartTextPromptCacheBreakpointParam(),
	}
	toolPart := openai.ChatCompletionContentPartTextParam{
		Text:                  "stable tool output",
		PromptCacheBreakpoint: openai.NewChatCompletionContentPartTextPromptCacheBreakpointParam(),
	}
	in := &openai.ChatCompletionNewParams{
		Model: "gpt-test",
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage([]openai.ChatCompletionContentPartTextParam{systemPart}),
			openai.UserMessage([]openai.ChatCompletionContentPartUnionParam{{OfText: &userPart}}),
			{
				OfTool: &openai.ChatCompletionToolMessageParam{
					ToolCallID: "call_1",
					Content: openai.ChatCompletionToolMessageParamContentUnion{
						OfArrayOfContentParts: []openai.ChatCompletionContentPartTextParam{toolPart},
					},
				},
			},
		},
		PromptCacheKey: openai.Opt("stable-key"),
		PromptCacheOptions: openai.ChatCompletionNewParamsPromptCacheOptions{
			Mode: "explicit",
			Ttl:  "30m",
		},
		PromptCacheRetention: openai.ChatCompletionNewParamsPromptCacheRetention24h,
	}

	responsesReq := ConvertChatToOpenAIResponses(in, 4096)
	require.Equal(t, "stable-key", responsesReq.PromptCacheKey.Value)
	require.Equal(t, "explicit", responsesReq.PromptCacheOptions.Mode)
	require.Equal(t, "30m", responsesReq.PromptCacheOptions.Ttl)
	require.Equal(t, responses.ResponseNewParamsPromptCacheRetention24h, responsesReq.PromptCacheRetention)
	require.False(t, responsesReq.Instructions.Valid())
	require.Len(t, responsesReq.Input.OfInputItemList, 3)
	requireResponsesTextBreakpoint(t, responsesReq.Input.OfInputItemList[0], "system")
	requireResponsesTextBreakpoint(t, responsesReq.Input.OfInputItemList[1], "user")
	toolOutput := responsesReq.Input.OfInputItemList[2].OfFunctionCallOutput
	require.NotNil(t, toolOutput)
	require.Len(t, toolOutput.Output.OfResponseFunctionCallOutputItemArray, 1)
	require.False(t, openaiparam.IsOmitted(
		toolOutput.Output.OfResponseFunctionCallOutputItemArray[0].OfInputText.PromptCacheBreakpoint))

	out := ConvertOpenAIResponsesToChat(responsesReq, 4096)
	require.Equal(t, "stable-key", out.PromptCacheKey.Value)
	require.Equal(t, "explicit", out.PromptCacheOptions.Mode)
	require.Equal(t, "30m", out.PromptCacheOptions.Ttl)
	require.Equal(t, openai.ChatCompletionNewParamsPromptCacheRetention24h, out.PromptCacheRetention)
	require.Len(t, out.Messages, 3)
	require.False(t, openaiparam.IsOmitted(
		out.Messages[0].OfSystem.Content.OfArrayOfContentParts[0].PromptCacheBreakpoint))
	require.False(t, openaiparam.IsOmitted(
		out.Messages[1].OfUser.Content.OfArrayOfContentParts[0].OfText.PromptCacheBreakpoint))
	require.False(t, openaiparam.IsOmitted(
		out.Messages[2].OfTool.Content.OfArrayOfContentParts[0].PromptCacheBreakpoint))
}

func requireResponsesTextBreakpoint(
	t *testing.T,
	item responses.ResponseInputItemUnionParam,
	role string,
) {
	t.Helper()
	require.NotNil(t, item.OfMessage)
	require.Equal(t, role, string(item.OfMessage.Role))
	require.Len(t, item.OfMessage.Content.OfInputItemContentList, 1)
	require.NotNil(t, item.OfMessage.Content.OfInputItemContentList[0].OfInputText)
	require.False(t, openaiparam.IsOmitted(
		item.OfMessage.Content.OfInputItemContentList[0].OfInputText.PromptCacheBreakpoint))
}
