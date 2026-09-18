package request

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicparam "github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/openai/openai-go/v3"
	openaiparam "github.com/openai/openai-go/v3/packages/param"
	"github.com/stretchr/testify/require"
)

func TestCacheControlProtocolFamilyPreservesMultiblockBoundaries(t *testing.T) {
	userText := anthropic.NewTextBlock("user text")
	userImage := anthropic.NewImageBlock(anthropic.URLImageSourceParam{
		URL: "https://example.com/image.png",
	})
	userImage.OfImage.CacheControl = anthropic.NewCacheControlEphemeralParam()
	assistantText := anthropic.NewTextBlock("assistant text")
	assistantText.OfText.CacheControl = anthropic.NewCacheControlEphemeralParam()

	in := &anthropic.MessageNewParams{
		Model:     "claude-test",
		MaxTokens: 256,
		System: []anthropic.TextBlockParam{
			{Text: "system part one"},
			{
				Text:         "system part two",
				CacheControl: anthropic.NewCacheControlEphemeralParam(),
			},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(userText, userImage),
			anthropic.NewAssistantMessage(assistantText),
		},
	}

	t.Run("messages chat messages", func(t *testing.T) {
		chat, _ := ConvertAnthropicToOpenAIRequest(in, true, true, false)
		require.Equal(t, "explicit", chat.PromptCacheOptions.Mode)
		require.Len(t, chat.Messages, 3)

		systemParts := chat.Messages[0].OfSystem.Content.OfArrayOfContentParts
		require.Len(t, systemParts, 2)
		require.True(t, openaiparam.IsOmitted(systemParts[0].PromptCacheBreakpoint))
		require.False(t, openaiparam.IsOmitted(systemParts[1].PromptCacheBreakpoint))

		userParts := chat.Messages[1].OfUser.Content.OfArrayOfContentParts
		require.Len(t, userParts, 2)
		require.NotNil(t, userParts[0].OfText)
		require.True(t, openaiparam.IsOmitted(userParts[0].OfText.PromptCacheBreakpoint))
		require.NotNil(t, userParts[1].OfImageURL)
		require.False(t, openaiparam.IsOmitted(userParts[1].OfImageURL.PromptCacheBreakpoint))

		assistantParts := chat.Messages[2].OfAssistant.Content.OfArrayOfContentParts
		require.Len(t, assistantParts, 1)
		require.False(t, openaiparam.IsOmitted(assistantParts[0].OfText.PromptCacheBreakpoint))

		out := ConvertOpenAIToAnthropicRequest(chat, 4096)
		requireAnthropicFamilyBoundaries(t, out)
	})

	t.Run("messages responses messages", func(t *testing.T) {
		responsesReq := ConvertAnthropicV1ToResponsesRequest(in)
		require.Equal(t, "explicit", responsesReq.PromptCacheOptions.Mode)
		require.Len(t, responsesReq.Input.OfInputItemList, 3)

		system := responsesReq.Input.OfInputItemList[0].OfMessage
		require.NotNil(t, system)
		require.Len(t, system.Content.OfInputItemContentList, 2)
		require.True(t, openaiparam.IsOmitted(
			system.Content.OfInputItemContentList[0].OfInputText.PromptCacheBreakpoint))
		require.False(t, openaiparam.IsOmitted(
			system.Content.OfInputItemContentList[1].OfInputText.PromptCacheBreakpoint))

		user := responsesReq.Input.OfInputItemList[1].OfMessage
		require.NotNil(t, user)
		require.Len(t, user.Content.OfInputItemContentList, 2)
		require.True(t, openaiparam.IsOmitted(
			user.Content.OfInputItemContentList[0].OfInputText.PromptCacheBreakpoint))
		require.False(t, openaiparam.IsOmitted(
			user.Content.OfInputItemContentList[1].OfInputImage.PromptCacheBreakpoint))

		assistant := responsesReq.Input.OfInputItemList[2].OfOutputMessage
		require.NotNil(t, assistant)
		require.Len(t, assistant.Content, 1)
		require.NotNil(t, assistant.Content[0].OfOutputText)
		require.Equal(t, "assistant text", assistant.Content[0].OfOutputText.Text)

		out := ConvertOpenAIResponsesToAnthropicBetaRequest(*responsesReq, 4096)
		// A prompt-cache breakpoint has no output_text equivalent (the
		// Responses API only accepts breakpoints on input_text/input_image,
		// which are only valid on user/system content) — the assistant
		// boundary is necessarily dropped on this round trip, unlike the
		// chat/messages family above where it survives.
		requireAnthropicFamilyBoundariesAssistantBreakpointDropped(t, out)
	})
}

func TestCacheControlProtocolFamilyDoesNotSynthesizeBreakpoints(t *testing.T) {
	anthropicReq := &anthropic.MessageNewParams{
		Model:     "claude-test",
		MaxTokens: 128,
		System:    []anthropic.TextBlockParam{{Text: "system"}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
		},
	}

	// The content-part list is the one shape a converted message ever takes —
	// see the cache-shape invariant in cache_control.go. No breakpoint is
	// synthesized onto it.
	chat, _ := ConvertAnthropicToOpenAIRequest(anthropicReq, true, false, false)
	require.Empty(t, chat.PromptCacheOptions.Mode)
	chatSystem := chat.Messages[0].OfSystem.Content.OfArrayOfContentParts
	require.Len(t, chatSystem, 1)
	require.True(t, openaiparam.IsOmitted(chatSystem[0].PromptCacheBreakpoint))
	chatUser := chat.Messages[1].OfUser.Content.OfArrayOfContentParts
	require.Len(t, chatUser, 1)
	require.True(t, openaiparam.IsOmitted(chatUser[0].OfText.PromptCacheBreakpoint))

	responsesReq := ConvertAnthropicV1ToResponsesRequest(anthropicReq)
	require.Empty(t, responsesReq.PromptCacheOptions.Mode)
	require.True(t, responsesReq.Instructions.Valid())
	require.Len(t, responsesReq.Input.OfInputItemList, 1)
	// The content-part list is the one shape a converted message ever takes —
	// see the cache-shape invariant in cache_control.go. No breakpoint is
	// synthesized onto it.
	userParts := responsesReq.Input.OfInputItemList[0].OfMessage.Content.OfInputItemContentList
	require.Len(t, userParts, 1)
	require.True(t, openaiparam.IsOmitted(userParts[0].OfInputText.PromptCacheBreakpoint))

	// Responses→Chat collapses text content to the compact string form
	// unconditionally, so its shape does not depend on breakpoints either.
	chatAgain := ConvertOpenAIResponsesToChat(responsesReq, 4096)
	require.Empty(t, chatAgain.PromptCacheOptions.Mode)
	require.True(t, chatAgain.Messages[0].OfSystem.Content.OfString.Valid())
	require.True(t, chatAgain.Messages[1].OfUser.Content.OfString.Valid())
}

func TestCacheControlProtocolFamilyToolFallbackPrefersSystemPrefix(t *testing.T) {
	in := &anthropic.MessageNewParams{
		Model:     "claude-test",
		MaxTokens: 128,
		System:    []anthropic.TextBlockParam{{Text: "stable system"}},
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
			anthropic.NewUserMessage(anthropic.NewTextBlock("dynamic user input")),
		},
	}

	chat, _ := ConvertAnthropicToOpenAIRequest(in, true, false, false)
	require.Equal(t, "explicit", chat.PromptCacheOptions.Mode)
	require.Len(t, chat.Messages[0].OfSystem.Content.OfArrayOfContentParts, 1)
	require.False(t, openaiparam.IsOmitted(
		chat.Messages[0].OfSystem.Content.OfArrayOfContentParts[0].PromptCacheBreakpoint))
	require.Len(t, chat.Messages[1].OfUser.Content.OfArrayOfContentParts, 1)

	responsesReq := ConvertAnthropicV1ToResponsesRequest(in)
	require.Equal(t, "explicit", responsesReq.PromptCacheOptions.Mode)
	require.False(t, responsesReq.Instructions.Valid())
	require.Len(t, responsesReq.Input.OfInputItemList, 2)
	requireResponsesTextBreakpoint(t, responsesReq.Input.OfInputItemList[0], "system")
	userParts := responsesReq.Input.OfInputItemList[1].OfMessage.Content.OfInputItemContentList
	require.Len(t, userParts, 1)
	require.True(t, openaiparam.IsOmitted(userParts[0].OfInputText.PromptCacheBreakpoint))
}

func TestCacheControlProtocolFamilyToolUseFallbackPrefersSystemPrefix(t *testing.T) {
	toolUse := anthropic.NewToolUseBlock("call_1", map[string]any{"query": "weather"}, "lookup")
	toolUse.OfToolUse.CacheControl = anthropic.NewCacheControlEphemeralParam()
	in := &anthropic.MessageNewParams{
		Model:     "claude-test",
		MaxTokens: 128,
		System:    []anthropic.TextBlockParam{{Text: "stable system"}},
		Messages: []anthropic.MessageParam{
			anthropic.NewAssistantMessage(toolUse),
		},
	}

	chat, _ := ConvertAnthropicToOpenAIRequest(in, true, false, false)
	require.Equal(t, "explicit", chat.PromptCacheOptions.Mode)
	require.Len(t, chat.Messages[0].OfSystem.Content.OfArrayOfContentParts, 1)
	require.False(t, openaiparam.IsOmitted(
		chat.Messages[0].OfSystem.Content.OfArrayOfContentParts[0].PromptCacheBreakpoint))

	responsesReq := ConvertAnthropicV1ToResponsesRequest(in)
	require.Equal(t, "explicit", responsesReq.PromptCacheOptions.Mode)
	require.False(t, responsesReq.Instructions.Valid())
	require.Len(t, responsesReq.Input.OfInputItemList, 2)
	requireResponsesTextBreakpoint(t, responsesReq.Input.OfInputItemList[0], "system")
	require.NotNil(t, responsesReq.Input.OfInputItemList[1].OfFunctionCall)

	betaToolUse := anthropic.NewBetaToolUseBlock("call_1", map[string]any{"query": "weather"}, "lookup")
	betaToolUse.OfToolUse.CacheControl = anthropic.NewBetaCacheControlEphemeralParam()
	betaIn := &anthropic.BetaMessageNewParams{
		Model:     "claude-test",
		MaxTokens: 128,
		System:    []anthropic.BetaTextBlockParam{{Text: "stable system"}},
		Messages: []anthropic.BetaMessageParam{
			{
				Role:    anthropic.BetaMessageParamRoleAssistant,
				Content: []anthropic.BetaContentBlockParamUnion{betaToolUse},
			},
		},
	}

	betaResponsesReq := ConvertAnthropicBetaToResponsesRequest(betaIn)
	require.Equal(t, "explicit", betaResponsesReq.PromptCacheOptions.Mode)
	require.False(t, betaResponsesReq.Instructions.Valid())
	require.Len(t, betaResponsesReq.Input.OfInputItemList, 2)
	requireResponsesTextBreakpoint(t, betaResponsesReq.Input.OfInputItemList[0], "system")
	require.NotNil(t, betaResponsesReq.Input.OfInputItemList[1].OfFunctionCall)
}

func requireAnthropicFamilyBoundaries(t *testing.T, out *anthropic.BetaMessageNewParams) {
	t.Helper()
	require.Len(t, out.System, 2)
	require.True(t, anthropicparam.IsOmitted(out.System[0].CacheControl))
	require.False(t, anthropicparam.IsOmitted(out.System[1].CacheControl))
	require.Len(t, out.Messages, 2)

	user := out.Messages[0]
	require.Len(t, user.Content, 2)
	require.NotNil(t, user.Content[0].OfText)
	require.True(t, anthropicparam.IsOmitted(user.Content[0].OfText.CacheControl))
	require.NotNil(t, user.Content[1].OfImage)
	require.False(t, anthropicparam.IsOmitted(user.Content[1].OfImage.CacheControl))

	assistant := out.Messages[1]
	require.Len(t, assistant.Content, 1)
	require.NotNil(t, assistant.Content[0].OfText)
	require.False(t, anthropicparam.IsOmitted(assistant.Content[0].OfText.CacheControl))
}

// requireAnthropicFamilyBoundariesAssistantBreakpointDropped is
// requireAnthropicFamilyBoundaries, except it does not expect the assistant
// text's cache_control to survive — see its caller for why.
func requireAnthropicFamilyBoundariesAssistantBreakpointDropped(t *testing.T, out *anthropic.BetaMessageNewParams) {
	t.Helper()
	require.Len(t, out.System, 2)
	require.True(t, anthropicparam.IsOmitted(out.System[0].CacheControl))
	require.False(t, anthropicparam.IsOmitted(out.System[1].CacheControl))
	require.Len(t, out.Messages, 2)

	user := out.Messages[0]
	require.Len(t, user.Content, 2)
	require.NotNil(t, user.Content[0].OfText)
	require.True(t, anthropicparam.IsOmitted(user.Content[0].OfText.CacheControl))
	require.NotNil(t, user.Content[1].OfImage)
	require.False(t, anthropicparam.IsOmitted(user.Content[1].OfImage.CacheControl))

	assistant := out.Messages[1]
	require.Len(t, assistant.Content, 1)
	require.NotNil(t, assistant.Content[0].OfText)
	require.Equal(t, "assistant text", assistant.Content[0].OfText.Text)
	require.True(t, anthropicparam.IsOmitted(assistant.Content[0].OfText.CacheControl))
}

func TestCacheControlProtocolFamilyChatResponsesKeepsMultipleBreakpoints(t *testing.T) {
	first := openai.ChatCompletionContentPartTextParam{
		Text:                  "first",
		PromptCacheBreakpoint: openai.NewChatCompletionContentPartTextPromptCacheBreakpointParam(),
	}
	second := openai.ChatCompletionContentPartTextParam{
		Text:                  "second",
		PromptCacheBreakpoint: openai.NewChatCompletionContentPartTextPromptCacheBreakpointParam(),
	}
	in := &openai.ChatCompletionNewParams{
		Model: "gpt-test",
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage([]openai.ChatCompletionContentPartUnionParam{
				{OfText: &first},
				{OfText: &second},
			}),
		},
		PromptCacheOptions: openai.ChatCompletionNewParamsPromptCacheOptions{Mode: "explicit"},
	}

	responsesReq := ConvertChatToOpenAIResponses(in, 4096)
	content := responsesReq.Input.OfInputItemList[0].OfMessage.Content.OfInputItemContentList
	require.Len(t, content, 2)
	require.False(t, openaiparam.IsOmitted(content[0].OfInputText.PromptCacheBreakpoint))
	require.False(t, openaiparam.IsOmitted(content[1].OfInputText.PromptCacheBreakpoint))

	out := ConvertOpenAIResponsesToChat(responsesReq, 4096)
	parts := out.Messages[0].OfUser.Content.OfArrayOfContentParts
	require.Len(t, parts, 2)
	require.False(t, openaiparam.IsOmitted(parts[0].OfText.PromptCacheBreakpoint))
	require.False(t, openaiparam.IsOmitted(parts[1].OfText.PromptCacheBreakpoint))
}
