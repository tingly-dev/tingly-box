package server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// SDKProbeBuilder builds SDK requests for probe operations
type SDKProbeBuilder struct{}

// NewSDKProbeBuilder creates a new SDK probe builder
func NewSDKProbeBuilder() *SDKProbeBuilder {
	return &SDKProbeBuilder{}
}

// buildAnthropicMessageRequest builds an Anthropic MessageNewParams for probing
func (b *SDKProbeBuilder) buildAnthropicMessageRequest(model, message string, testMode ProbeV2TestMode) anthropic.MessageNewParams {
	systemMessages := []anthropic.TextBlockParam{
		{
			Text: "work as `echo`",
		},
	}

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(message)),
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: 1024,
		System:    systemMessages,
		Messages:  messages,
	}

	if testMode == ProbeV2ModeTool {
		params.Tools = GetProbeToolsAnthropic()
		params.ToolChoice = GetProbeToolChoiceAutoAnthropic()
	}

	return params
}

// buildOpenAIChatRequest builds an OpenAI ChatCompletionNewParams for probing
func (b *SDKProbeBuilder) buildOpenAIChatRequest(model, message string, testMode ProbeV2TestMode) openai.ChatCompletionNewParams {
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage("work as `echo`"),
		openai.UserMessage(message),
	}

	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(model),
		Messages: messages,
	}

	return params
}

// SDKResponseConverter converts SDK responses to ProbeV2Data
type SDKResponseConverter struct{}

// NewSDKResponseConverter creates a new SDK response converter
func NewSDKResponseConverter() *SDKResponseConverter {
	return &SDKResponseConverter{}
}

// convertAnthropicMessageToProbeV2Data converts an Anthropic Message response to ProbeV2Data
func (c *SDKResponseConverter) convertAnthropicMessageToProbeV2Data(resp *anthropic.Message, latencyMs int64, url string) *ProbeV2Data {
	data := &ProbeV2Data{
		LatencyMs:  latencyMs,
		RequestURL: url,
	}

	var content string
	var toolCalls []ProbeV2ToolCall

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			content += string(block.Text)
		case "tool_use":
			var args map[string]interface{}
			if len(block.Input) > 0 {
				json.Unmarshal(block.Input, &args)
			}
			toolCalls = append(toolCalls, ProbeV2ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: args,
			})
		}
	}

	data.Content = content
	data.ToolCalls = toolCalls

	if resp.Usage.InputTokens != 0 || resp.Usage.OutputTokens != 0 {
		data.Usage = &ProbeV2Usage{
			PromptTokens:     int(resp.Usage.InputTokens),
			CompletionTokens: int(resp.Usage.OutputTokens),
			TotalTokens:      int(resp.Usage.InputTokens + resp.Usage.OutputTokens),
		}
	}

	return data
}

// convertOpenAIChatCompletionToProbeV2Data converts an OpenAI ChatCompletion response to ProbeV2Data
func (c *SDKResponseConverter) convertOpenAIChatCompletionToProbeV2Data(resp *openai.ChatCompletion, latencyMs int64, url string) *ProbeV2Data {
	data := &ProbeV2Data{
		LatencyMs:  latencyMs,
		RequestURL: url,
	}

	var content string
	var toolCalls []ProbeV2ToolCall

	if len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content

		for _, tc := range resp.Choices[0].Message.ToolCalls {
			var args map[string]interface{}
			if tc.Function.Arguments != "" {
				json.Unmarshal([]byte(tc.Function.Arguments), &args)
			}
			toolCalls = append(toolCalls, ProbeV2ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: args,
			})
		}
	}

	data.Content = content
	data.ToolCalls = toolCalls

	if resp.Usage.PromptTokens != 0 || resp.Usage.CompletionTokens != 0 || resp.Usage.TotalTokens != 0 {
		data.Usage = &ProbeV2Usage{
			PromptTokens:     int(resp.Usage.PromptTokens),
			CompletionTokens: int(resp.Usage.CompletionTokens),
			TotalTokens:      int(resp.Usage.TotalTokens),
		}
	}

	return data
}

// collectAnthropicStream collects all chunks from an Anthropic streaming response
func (c *SDKResponseConverter) collectAnthropicStream(ctx context.Context, anthropicClient *client.AnthropicClient, params anthropic.MessageNewParams, url string) (*ProbeV2Data, error) {
	startTime := time.Now()

	stream := anthropicClient.MessagesNewStreaming(ctx, params)
	defer stream.Close()

	var content string
	var toolCalls []ProbeV2ToolCall
	var usage *ProbeV2Usage

	for stream.Next() {
		event := stream.Current()

		switch event.Type {
		case "content_block_delta":
			if event.Delta.Type == "text_delta" && event.Delta.Text != "" {
				content += event.Delta.Text
			}
		case "message_stop":
			if event.Usage.InputTokens != 0 || event.Usage.OutputTokens != 0 {
				usage = &ProbeV2Usage{
					PromptTokens:     int(event.Usage.InputTokens),
					CompletionTokens: int(event.Usage.OutputTokens),
					TotalTokens:      int(event.Usage.InputTokens + event.Usage.OutputTokens),
				}
			}
		}
	}

	if err := stream.Err(); err != nil {
		return nil, err
	}

	return &ProbeV2Data{
		Content:    content,
		ToolCalls:  toolCalls,
		Usage:      usage,
		LatencyMs:  time.Since(startTime).Milliseconds(),
		RequestURL: url,
	}, nil
}

// collectOpenAIStream collects all chunks from an OpenAI streaming response
func (c *SDKResponseConverter) collectOpenAIStream(ctx context.Context, openaiClient *client.OpenAIClient, params openai.ChatCompletionNewParams, url string) (*ProbeV2Data, error) {
	startTime := time.Now()

	stream := openaiClient.ChatCompletionsNewStreaming(ctx, params)
	defer stream.Close()

	var content string
	var toolCallsMap = make(map[int]*ProbeV2ToolCall)
	var usage *ProbeV2Usage

	for stream.Next() {
		chunk := stream.Current()

		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta

			if delta.Content != "" {
				content += delta.Content
			}

			for _, tc := range delta.ToolCalls {
				idx := int(tc.Index)
				if _, exists := toolCallsMap[idx]; !exists {
					toolCallsMap[idx] = &ProbeV2ToolCall{}
				}
				if tc.ID != "" {
					toolCallsMap[idx].ID = tc.ID
				}
				if tc.Function.Name != "" {
					toolCallsMap[idx].Name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					// Accumulate arguments
				}
			}
		}

		if chunk.Usage.PromptTokens != 0 || chunk.Usage.CompletionTokens != 0 {
			usage = &ProbeV2Usage{
				PromptTokens:     int(chunk.Usage.PromptTokens),
				CompletionTokens: int(chunk.Usage.CompletionTokens),
				TotalTokens:      int(chunk.Usage.PromptTokens + chunk.Usage.CompletionTokens),
			}
		}
	}

	if err := stream.Err(); err != nil {
		return nil, err
	}

	// Convert map to slice
	var toolCalls []ProbeV2ToolCall
	for _, tc := range toolCallsMap {
		toolCalls = append(toolCalls, *tc)
	}

	return &ProbeV2Data{
		Content:    content,
		ToolCalls:  toolCalls,
		Usage:      usage,
		LatencyMs:  time.Since(startTime).Milliseconds(),
		RequestURL: url,
	}, nil
}

// getClientForProvider gets the appropriate SDK client for a provider
func (s *Server) getClientForProvider(provider *typ.Provider, model string) (interface{}, error) {
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		client := s.clientPool.GetAnthropicClient(provider, model)
		if client == nil {
			return nil, fmt.Errorf("failed to get Anthropic client for provider: %s", provider.Name)
		}
		return client, nil
	case protocol.APIStyleOpenAI:
		client := s.clientPool.GetOpenAIClient(provider, model)
		if client == nil {
			return nil, fmt.Errorf("failed to get OpenAI client for provider: %s", provider.Name)
		}
		return client, nil
	case protocol.APIStyleGoogle:
		client := s.clientPool.GetGoogleClient(provider, model)
		if client == nil {
			return nil, fmt.Errorf("failed to get Google client for provider: %s", provider.Name)
		}
		return client, nil
	default:
		return nil, fmt.Errorf("unsupported API style: %s", provider.APIStyle)
	}
}

// probeProviderWithSDK performs a non-streaming probe for a provider using SDK
func (s *Server) probeProviderWithSDK(ctx context.Context, provider *typ.Provider, model, message string, testMode ProbeV2TestMode) (*ProbeV2Data, error) {
	startTime := time.Now()

	clientInterface, err := s.getClientForProvider(provider, model)
	if err != nil {
		return nil, err
	}

	builder := NewSDKProbeBuilder()
	converter := NewSDKResponseConverter()

	url := provider.APIBase
	if provider.APIStyle == protocol.APIStyleAnthropic {
		url += "/v1/messages"
	} else {
		url += "/v1/chat/completions"
	}

	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		anthropicClient := clientInterface.(*client.AnthropicClient)
		params := builder.buildAnthropicMessageRequest(model, message, testMode)
		resp, err := anthropicClient.MessagesNew(ctx, params)
		if err != nil {
			return nil, err
		}
		return converter.convertAnthropicMessageToProbeV2Data(resp, time.Since(startTime).Milliseconds(), url), nil

	case protocol.APIStyleOpenAI:
		openaiClient := clientInterface.(*client.OpenAIClient)
		params := builder.buildOpenAIChatRequest(model, message, testMode)
		resp, err := openaiClient.ChatCompletionsNew(ctx, params)
		if err != nil {
			return nil, err
		}
		return converter.convertOpenAIChatCompletionToProbeV2Data(resp, time.Since(startTime).Milliseconds(), url), nil

	default:
		return nil, fmt.Errorf("unsupported API style: %s", provider.APIStyle)
	}
}

// probeProviderWithSDKStreaming performs a streaming probe for a provider using SDK
func (s *Server) probeProviderWithSDKStreaming(ctx context.Context, provider *typ.Provider, model, message string, testMode ProbeV2TestMode) (*ProbeV2Data, error) {
	clientInterface, err := s.getClientForProvider(provider, model)
	if err != nil {
		return nil, err
	}

	builder := NewSDKProbeBuilder()
	converter := NewSDKResponseConverter()

	url := provider.APIBase
	if provider.APIStyle == protocol.APIStyleAnthropic {
		url += "/v1/messages"
	} else {
		url += "/v1/chat/completions"
	}

	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		anthropicClient := clientInterface.(*client.AnthropicClient)
		params := builder.buildAnthropicMessageRequest(model, message, testMode)
		return converter.collectAnthropicStream(ctx, anthropicClient, params, url)

	case protocol.APIStyleOpenAI:
		openaiClient := clientInterface.(*client.OpenAIClient)
		params := builder.buildOpenAIChatRequest(model, message, testMode)
		return converter.collectOpenAIStream(ctx, openaiClient, params, url)

	default:
		return nil, fmt.Errorf("unsupported API style: %s", provider.APIStyle)
	}
}
