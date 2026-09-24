package request

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file is the tool-channel companion of image_conversions_test.go
// (issue #1606): images returned by tools — OpenAI Chat tool messages with
// image_url parts, Anthropic tool_result blocks with image content, Responses
// function_call_output with input_image items — must survive every request
// conversion, exactly like their user-message counterparts.

const (
	toolImgDataURL = "data:image/png;base64,iVBORw0KGgo="
	toolImgHTTPURL = "https://example.com/screenshot.png"
)

// chatToolImageRequest builds, via JSON (the exact ingress path), a Chat
// Completions request whose tool message carries [text, image_url] content.
func chatToolImageRequest(t *testing.T) *openai.ChatCompletionNewParams {
	t.Helper()
	raw := []byte(`{
		"model": "test-model",
		"messages": [
			{"role": "user", "content": "analyze the image"},
			{"role": "assistant", "content": "", "tool_calls": [
				{"id": "call_1", "type": "function",
				 "function": {"name": "vision_analyze", "arguments": "{}"}}
			]},
			{"role": "tool", "tool_call_id": "call_1", "content": [
				{"type": "text", "text": "Image loaded."},
				{"type": "image_url", "image_url": {"url": "` + toolImgDataURL + `"}}
			]}
		]
	}`)
	req := &openai.ChatCompletionNewParams{}
	require.NoError(t, json.Unmarshal(raw, req))
	return req
}

// The SDK-level parse/marshal survival of tool image parts is locked by the
// fork's chatcompletion_toolmessage_patch_test.go and the gateway-level round
// trip in internal/protocol/multimodal_roundtrip_test.go; the tests here
// cover the conversions on top of the parsed request.

// TestConvertOpenAIToAnthropic_ToolMessageImage: Chat tool message with
// [text, image_url] must become an Anthropic tool_result whose content holds
// a text block AND an image block.
func TestConvertOpenAIToAnthropic_ToolMessageImage(t *testing.T) {
	out := ConvertOpenAIToAnthropicRequest(chatToolImageRequest(t), 1024)
	require.NotNil(t, out)

	var toolResult *anthropic.BetaToolResultBlockParam
	for _, msg := range out.Messages {
		for _, block := range msg.Content {
			if block.OfToolResult != nil {
				toolResult = block.OfToolResult
			}
		}
	}
	require.NotNil(t, toolResult, "tool message should become a tool_result block")
	require.Len(t, toolResult.Content, 2, "text + image content must both survive")

	require.NotNil(t, toolResult.Content[0].OfText)
	assert.Equal(t, "Image loaded.", toolResult.Content[0].OfText.Text)

	require.NotNil(t, toolResult.Content[1].OfImage, "image part must convert to an image block")
	src := toolResult.Content[1].OfImage.Source
	require.NotNil(t, src.OfBase64)
	assert.Equal(t, "image/png", string(src.OfBase64.MediaType))
	assert.Equal(t, "iVBORw0KGgo=", src.OfBase64.Data)
}

// TestConvertChatToOpenAIResponses_ToolMessageImage: Chat tool message with
// image content must become a function_call_output whose output item list
// carries input_text + input_image.
func TestConvertChatToOpenAIResponses_ToolMessageImage(t *testing.T) {
	out := ConvertChatToOpenAIResponses(chatToolImageRequest(t), 1024)
	require.NotNil(t, out)

	var fco *responses.ResponseInputItemFunctionCallOutputParam
	for _, item := range out.Input.OfInputItemList {
		if item.OfFunctionCallOutput != nil {
			fco = item.OfFunctionCallOutput
		}
	}
	require.NotNil(t, fco)

	items := fco.Output.OfResponseFunctionCallOutputItemArray
	require.Len(t, items, 2, "text + image output items must both survive, got output: %+v", fco.Output)

	require.NotNil(t, items[0].OfInputText)
	assert.Equal(t, "Image loaded.", items[0].OfInputText.Text)

	require.NotNil(t, items[1].OfInputImage, "image part must convert to input_image")
	assert.Equal(t, toolImgDataURL, items[1].OfInputImage.ImageURL.Value)
}

// TestConvertOpenAIResponsesToChat_FunctionCallOutputImage: a Responses
// function_call_output with [input_text, input_image] must become a Chat tool
// message with [text, image_url] parts.
func TestConvertOpenAIResponsesToChat_FunctionCallOutputImage(t *testing.T) {
	params := &responses.ResponseNewParams{
		Model:           "test-model",
		MaxOutputTokens: param.NewOpt(int64(100)),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: responses.ResponseInputParam{
				{OfFunctionCall: &responses.ResponseFunctionToolCallParam{
					CallID: "call_1", Name: "vision_analyze", Arguments: "{}",
				}},
				{OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
					CallID: param.NewOpt("call_1"),
					Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
						OfResponseFunctionCallOutputItemArray: responses.ResponseFunctionCallOutputItemListParam{
							{OfInputText: &responses.ResponseInputTextContentParam{Text: "Image loaded."}},
							{OfInputImage: &responses.ResponseInputImageContentParam{
								ImageURL: param.NewOpt(toolImgDataURL),
							}},
						},
					},
				}},
			},
		},
	}

	out := ConvertOpenAIResponsesToChat(t.Context(), params, 1024)
	require.NotNil(t, out)
	assertOpenAIToolMessageHasImage(t, out, "Image loaded.")
}

// anthropicV1ToolResultImageRequest builds an Anthropic v1 request whose
// tool_result content holds [text, image].
func anthropicV1ToolResultImageRequest() *anthropic.MessageNewParams {
	return &anthropic.MessageNewParams{
		Model:     "test-model",
		MaxTokens: 100,
		Messages: []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleAssistant,
				Content: []anthropic.ContentBlockParamUnion{
					anthropic.NewToolUseBlock("toolu_1", map[string]any{}, "screenshot"),
				},
			},
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					{OfToolResult: &anthropic.ToolResultBlockParam{
						ToolUseID: "toolu_1",
						Content: []anthropic.ToolResultBlockParamContentUnion{
							{OfText: &anthropic.TextBlockParam{Text: "took screenshot"}},
							{OfImage: &anthropic.ImageBlockParam{
								Source: anthropic.ImageBlockParamSourceUnion{
									OfBase64: &anthropic.Base64ImageSourceParam{
										MediaType: "image/png",
										Data:      "iVBORw0KGgo=",
									},
								},
							}},
						},
					}},
				},
			},
		},
	}
}

// anthropicBetaToolResultImageRequest is the beta twin of the v1 fixture.
func anthropicBetaToolResultImageRequest() *anthropic.BetaMessageNewParams {
	return &anthropic.BetaMessageNewParams{
		Model:     "test-model",
		MaxTokens: 100,
		Messages: []anthropic.BetaMessageParam{
			{
				Role: anthropic.BetaMessageParamRoleAssistant,
				Content: []anthropic.BetaContentBlockParamUnion{
					anthropic.NewBetaToolUseBlock("toolu_1", map[string]any{}, "screenshot"),
				},
			},
			{
				Role: anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfToolResult: &anthropic.BetaToolResultBlockParam{
						ToolUseID: "toolu_1",
						Content: []anthropic.BetaToolResultBlockParamContentUnion{
							{OfText: &anthropic.BetaTextBlockParam{Text: "took screenshot"}},
							{OfImage: &anthropic.BetaImageBlockParam{
								Source: anthropic.BetaImageBlockParamSourceUnion{
									OfBase64: &anthropic.BetaBase64ImageSourceParam{
										MediaType: "image/png",
										Data:      "iVBORw0KGgo=",
									},
								},
							}},
						},
					}},
				},
			},
		},
	}
}

// assertOpenAIToolMessageHasImage marshals the OpenAI request and asserts the
// tool message carries [text, image_url] content parts with the data URL.
func assertOpenAIToolMessageHasImage(t *testing.T, out *openai.ChatCompletionNewParams, expectedText string) {
	t.Helper()
	var tool *openai.ChatCompletionToolMessageParam
	for _, msg := range out.Messages {
		if msg.OfTool != nil {
			tool = msg.OfTool
		}
	}
	require.NotNil(t, tool, "expected a tool message")

	raw, err := json.Marshal(openai.ChatCompletionMessageParamUnion{OfTool: tool})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	parts, ok := m["content"].([]any)
	require.True(t, ok, "tool content should be a content-part array, got %v", m["content"])
	require.Len(t, parts, 2, "text + image parts must both survive")

	text := parts[0].(map[string]any)
	assert.Equal(t, "text", text["type"])
	assert.Equal(t, expectedText, text["text"])

	img := parts[1].(map[string]any)
	assert.Equal(t, "image_url", img["type"])
	imgURL, ok := img["image_url"].(map[string]any)
	require.True(t, ok, "image_url field must survive, got %v", img)
	assert.Equal(t, toolImgDataURL, imgURL["url"])
}

// TestConvertAnthropicV1ToOpenAI_ToolResultImage: v1 tool_result images must
// be forwarded as image_url parts on the OpenAI tool message (the Claude Code
// screenshot-tool shape).
func TestConvertAnthropicV1ToOpenAI_ToolResultImage(t *testing.T) {
	out, _ := ConvertAnthropicToOpenAIRequest(anthropicV1ToolResultImageRequest(), false, false, false)
	assertOpenAIToolMessageHasImage(t, out, "took screenshot")
}

// TestConvertAnthropicBetaToOpenAI_ToolResultImage is the beta twin.
func TestConvertAnthropicBetaToOpenAI_ToolResultImage(t *testing.T) {
	out, _ := ConvertAnthropicBetaToOpenAIRequest(anthropicBetaToolResultImageRequest(), false, false, false)
	assertOpenAIToolMessageHasImage(t, out, "took screenshot")
}

// assertResponsesFunctionCallOutputHasImage asserts the function_call_output
// output item list carries input_text + input_image.
func assertResponsesFunctionCallOutputHasImage(t *testing.T, out *responses.ResponseNewParams) {
	t.Helper()
	var fco *responses.ResponseInputItemFunctionCallOutputParam
	for _, item := range out.Input.OfInputItemList {
		if item.OfFunctionCallOutput != nil {
			fco = item.OfFunctionCallOutput
		}
	}
	require.NotNil(t, fco, "expected a function_call_output item")

	items := fco.Output.OfResponseFunctionCallOutputItemArray
	require.Len(t, items, 2, "text + image output items must both survive, got output: %+v", fco.Output)

	require.NotNil(t, items[0].OfInputText)
	assert.Equal(t, "took screenshot", items[0].OfInputText.Text)

	require.NotNil(t, items[1].OfInputImage, "image content must convert to input_image")
	assert.Equal(t, toolImgDataURL, items[1].OfInputImage.ImageURL.Value)
}

// TestConvertAnthropicV1ToResponses_ToolResultImage: v1 tool_result images →
// Responses input_image output items.
func TestConvertAnthropicV1ToResponses_ToolResultImage(t *testing.T) {
	out := ConvertAnthropicV1ToResponsesRequest(anthropicV1ToolResultImageRequest())
	require.NotNil(t, out)
	assertResponsesFunctionCallOutputHasImage(t, out)
}

// TestConvertAnthropicBetaToResponses_ToolResultImage is the beta twin.
func TestConvertAnthropicBetaToResponses_ToolResultImage(t *testing.T) {
	out := ConvertAnthropicBetaToResponsesRequest(anthropicBetaToolResultImageRequest())
	require.NotNil(t, out)
	assertResponsesFunctionCallOutputHasImage(t, out)
}

// TestConvertAnthropicV1ToOpenAI_ImageAlongsideToolResult: an image block that
// sits NEXT TO a tool_result in the same user message must not be dropped; it
// belongs on the follow-up user message.
func TestConvertAnthropicV1ToOpenAI_ImageAlongsideToolResult(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Model:     "test-model",
		MaxTokens: 100,
		Messages: []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					{OfToolResult: &anthropic.ToolResultBlockParam{
						ToolUseID: "toolu_1",
						Content: []anthropic.ToolResultBlockParamContentUnion{
							{OfText: &anthropic.TextBlockParam{Text: "done"}},
						},
					}},
					anthropic.NewTextBlock("compare with this:"),
					anthropic.NewImageBlockBase64("image/png", "iVBORw0KGgo="),
				},
			},
		},
	}

	out, _ := ConvertAnthropicToOpenAIRequest(req, false, false, false)

	var user *openai.ChatCompletionUserMessageParam
	for _, msg := range out.Messages {
		if msg.OfUser != nil {
			user = msg.OfUser
		}
	}
	require.NotNil(t, user, "text+image beside tool_result should yield a user message")

	parts := user.Content.OfArrayOfContentParts
	require.Len(t, parts, 2, "text + image parts must both survive")
	require.NotNil(t, parts[0].OfText)
	assert.Equal(t, "compare with this:", parts[0].OfText.Text)
	require.NotNil(t, parts[1].OfImageURL, "image beside tool_result must survive")
	assert.Equal(t, toolImgDataURL, parts[1].OfImageURL.ImageURL.URL)
}
