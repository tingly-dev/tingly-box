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

// TestParseImageURLToAnthropicSource covers the data-URL parser used by every
// "OpenAI image_url -> Anthropic image" conversion path.
func TestParseImageURLToAnthropicSource(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		mediaType string
		data      string
		remoteURL string
	}{
		{"data url", "data:image/png;base64,iVBORw0KGgo=", "image/png", "iVBORw0KGgo=", ""},
		{"https url", "https://example.com/cat.jpg", "", "", "https://example.com/cat.jpg"},
		{"empty", "", "", "", ""},
		{"non-base64 data url falls through as remoteURL", "data:image/png;utf8,abc", "", "", "data:image/png;utf8,abc"},
		{"malformed data url falls through", "data:broken", "", "", "data:broken"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mt, d, u := parseImageURLToAnthropicSource(tc.in)
			assert.Equal(t, tc.mediaType, mt)
			assert.Equal(t, tc.data, d)
			assert.Equal(t, tc.remoteURL, u)
		})
	}
}

// TestConvertOpenAIToAnthropic_ImageURL verifies that an OpenAI Chat
// Completions multipart user message with an image_url part is preserved as an
// Anthropic image content block.
func TestConvertOpenAIToAnthropic_ImageURL(t *testing.T) {
	// Build a multimodal user message via JSON (the same path the converter
	// uses internally) so we don't need to know the exact SDK union shape.
	rawMsg := json.RawMessage(`{
		"role": "user",
		"content": [
			{"type": "text", "text": "look"},
			{"type": "image_url", "image_url": {"url": "https://example.com/cat.png"}}
		]
	}`)
	var userMsg openai.ChatCompletionMessageParamUnion
	require.NoError(t, json.Unmarshal(rawMsg, &userMsg))

	req := &openai.ChatCompletionNewParams{
		Model:    openai.ChatModel("gpt-4o"),
		Messages: []openai.ChatCompletionMessageParamUnion{userMsg},
	}

	out := ConvertOpenAIToAnthropicRequest(req, 1024)
	require.Len(t, out.Messages, 1)
	require.Len(t, out.Messages[0].Content, 2, "expected text + image blocks")

	assert.NotNil(t, out.Messages[0].Content[0].OfText)
	assert.Equal(t, "look", out.Messages[0].Content[0].OfText.Text)

	require.NotNil(t, out.Messages[0].Content[1].OfImage)
	require.NotNil(t, out.Messages[0].Content[1].OfImage.Source.OfURL)
	assert.Equal(t, "https://example.com/cat.png", out.Messages[0].Content[1].OfImage.Source.OfURL.URL)
}

// TestConvertOpenAIToAnthropic_ImageDataURL verifies that an OpenAI data: URL
// becomes an Anthropic base64 image source.
func TestConvertOpenAIToAnthropic_ImageDataURL(t *testing.T) {
	rawMsg := json.RawMessage(`{
		"role": "user",
		"content": [
			{"type": "image_url", "image_url": {"url": "data:image/jpeg;base64,/9j/4AAQ"}}
		]
	}`)
	var userMsg openai.ChatCompletionMessageParamUnion
	require.NoError(t, json.Unmarshal(rawMsg, &userMsg))

	req := &openai.ChatCompletionNewParams{
		Model:    openai.ChatModel("gpt-4o"),
		Messages: []openai.ChatCompletionMessageParamUnion{userMsg},
	}

	out := ConvertOpenAIToAnthropicRequest(req, 1024)
	require.Len(t, out.Messages, 1)
	require.Len(t, out.Messages[0].Content, 1)

	require.NotNil(t, out.Messages[0].Content[0].OfImage)
	src := out.Messages[0].Content[0].OfImage.Source
	require.NotNil(t, src.OfBase64)
	assert.Equal(t, "image/jpeg", string(src.OfBase64.MediaType))
	assert.Equal(t, "/9j/4AAQ", src.OfBase64.Data)
}

// TestConvertAnthropicV1ToResponses_Image verifies Anthropic image -> Responses
// API input_image conversion (v1 path).
func TestConvertAnthropicV1ToResponses_Image(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Model:     "test-model",
		MaxTokens: 100,
		Messages: []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					anthropic.NewTextBlock("describe"),
					anthropic.NewImageBlockBase64("image/png", "iVBORw0KGgo="),
				},
			},
		},
	}

	out := ConvertAnthropicV1ToResponsesRequest(req)
	require.NotNil(t, out)
	require.Len(t, out.Input.OfInputItemList, 1)

	msg := out.Input.OfInputItemList[0].OfMessage
	require.NotNil(t, msg)
	require.True(t, !param.IsOmitted(msg.Content.OfInputItemContentList))
	parts := msg.Content.OfInputItemContentList
	require.Len(t, parts, 2)

	require.NotNil(t, parts[0].OfInputText)
	assert.Equal(t, "describe", parts[0].OfInputText.Text)

	require.NotNil(t, parts[1].OfInputImage)
	assert.Equal(t, "data:image/png;base64,iVBORw0KGgo=", parts[1].OfInputImage.ImageURL.Value)
}

// TestConvertAnthropicBetaToResponses_Image is the beta variant of the above.
func TestConvertAnthropicBetaToResponses_Image(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{
		Model:     "test-model",
		MaxTokens: 100,
		Messages: []anthropic.BetaMessageParam{
			{
				Role: anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{
					anthropic.NewBetaImageBlock(anthropic.BetaURLImageSourceParam{
						URL: "https://example.com/cat.png",
					}),
				},
			},
		},
	}

	out := ConvertAnthropicBetaToResponsesRequest(req)
	require.NotNil(t, out)
	require.Len(t, out.Input.OfInputItemList, 1)

	msg := out.Input.OfInputItemList[0].OfMessage
	require.NotNil(t, msg)
	parts := msg.Content.OfInputItemContentList
	require.Len(t, parts, 1)
	require.NotNil(t, parts[0].OfInputImage)
	assert.Equal(t, "https://example.com/cat.png", parts[0].OfInputImage.ImageURL.Value)
}

// TestConvertResponsesToAnthropic_Image verifies Responses input_image ->
// Anthropic image conversion for both v1 and beta paths.
func TestConvertResponsesToAnthropic_Image(t *testing.T) {
	makeReq := func() responses.ResponseNewParams {
		return responses.ResponseNewParams{
			Model:           "test-model",
			MaxOutputTokens: param.NewOpt(int64(100)),
			Input: responses.ResponseNewParamsInputUnion{
				OfInputItemList: responses.ResponseInputParam{
					{
						OfMessage: &responses.EasyInputMessageParam{
							Type: responses.EasyInputMessageTypeMessage,
							Role: responses.EasyInputMessageRole("user"),
							Content: responses.EasyInputMessageContentUnionParam{
								OfInputItemContentList: responses.ResponseInputMessageContentListParam{
									{OfInputText: &responses.ResponseInputTextParam{Text: "describe"}},
									{OfInputImage: &responses.ResponseInputImageParam{
										ImageURL: param.NewOpt("data:image/png;base64,iVBORw0KGgo="),
									}},
								},
							},
						},
					},
				},
			},
		}
	}

	t.Run("v1", func(t *testing.T) {
		out := ConvertOpenAIResponsesToAnthropicRequest(makeReq(), 1024)
		require.Len(t, out.Messages, 1)
		require.Len(t, out.Messages[0].Content, 2)

		assert.NotNil(t, out.Messages[0].Content[0].OfText)
		assert.Equal(t, "describe", out.Messages[0].Content[0].OfText.Text)

		require.NotNil(t, out.Messages[0].Content[1].OfImage)
		require.NotNil(t, out.Messages[0].Content[1].OfImage.Source.OfBase64)
		assert.Equal(t, "image/png", string(out.Messages[0].Content[1].OfImage.Source.OfBase64.MediaType))
	})

	t.Run("beta", func(t *testing.T) {
		out := ConvertOpenAIResponsesToAnthropicBetaRequest(makeReq(), 1024)
		require.Len(t, out.Messages, 1)
		require.Len(t, out.Messages[0].Content, 2)

		require.NotNil(t, out.Messages[0].Content[1].OfImage)
		require.NotNil(t, out.Messages[0].Content[1].OfImage.Source.OfBase64)
		assert.Equal(t, "image/png", string(out.Messages[0].Content[1].OfImage.Source.OfBase64.MediaType))
	})
}

// TestConvertChatToOpenAIResponses_Image verifies that a Chat Completions
// multipart user message with image_url is forwarded as input_image when
// converting to the Responses API.
func TestConvertChatToOpenAIResponses_Image(t *testing.T) {
	rawMsg := json.RawMessage(`{
		"role": "user",
		"content": [
			{"type": "text", "text": "look"},
			{"type": "image_url", "image_url": {"url": "https://example.com/cat.png"}}
		]
	}`)
	var userMsg openai.ChatCompletionMessageParamUnion
	require.NoError(t, json.Unmarshal(rawMsg, &userMsg))

	params := &openai.ChatCompletionNewParams{
		Model:    openai.ChatModel("gpt-4o"),
		Messages: []openai.ChatCompletionMessageParamUnion{userMsg},
	}

	out := ConvertChatToOpenAIResponses(params, 1024)
	require.NotNil(t, out)
	require.Len(t, out.Input.OfInputItemList, 1)

	msg := out.Input.OfInputItemList[0].OfMessage
	require.NotNil(t, msg)
	parts := msg.Content.OfInputItemContentList
	require.Len(t, parts, 2)

	require.NotNil(t, parts[0].OfInputText)
	assert.Equal(t, "look", parts[0].OfInputText.Text)

	require.NotNil(t, parts[1].OfInputImage)
	assert.Equal(t, "https://example.com/cat.png", parts[1].OfInputImage.ImageURL.Value)
}

// TestConvertOpenAIResponsesToChat_Image verifies that a Responses API
// multipart user message with input_image is forwarded as image_url when
// converting back to Chat Completions.
func TestConvertOpenAIResponsesToChat_Image(t *testing.T) {
	params := &responses.ResponseNewParams{
		Model:           "gpt-4o",
		MaxOutputTokens: param.NewOpt(int64(100)),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: responses.ResponseInputParam{
				{
					OfMessage: &responses.EasyInputMessageParam{
						Type: responses.EasyInputMessageTypeMessage,
						Role: responses.EasyInputMessageRole("user"),
						Content: responses.EasyInputMessageContentUnionParam{
							OfInputItemContentList: responses.ResponseInputMessageContentListParam{
								{OfInputText: &responses.ResponseInputTextParam{Text: "look"}},
								{OfInputImage: &responses.ResponseInputImageParam{
									ImageURL: param.NewOpt("https://example.com/cat.png"),
								}},
							},
						},
					},
				},
			},
		},
	}

	out := ConvertOpenAIResponsesToChat(params, 1024)
	require.NotNil(t, out)
	require.Len(t, out.Messages, 1)

	raw, err := json.Marshal(out.Messages[0])
	require.NoError(t, err)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &m))
	assert.Equal(t, "user", m["role"])

	parts, ok := m["content"].([]interface{})
	require.True(t, ok, "content should be an array, got: %v", m["content"])
	require.Len(t, parts, 2)

	first := parts[0].(map[string]interface{})
	assert.Equal(t, "text", first["type"])
	assert.Equal(t, "look", first["text"])

	second := parts[1].(map[string]interface{})
	assert.Equal(t, "image_url", second["type"])
	imgURL, ok := second["image_url"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "https://example.com/cat.png", imgURL["url"])
}
