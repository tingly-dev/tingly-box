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
func (b *SDKProbeBuilder) buildAnthropicMessageRequest(model, message string, testMode ProbeMode) anthropic.MessageNewParams {
	systemMessages := []anthropic.TextBlockParam{
		{
			Text: "work as `echo` if possible",
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
func (b *SDKProbeBuilder) buildOpenAIChatRequest(model, message string, testMode ProbeMode) openai.ChatCompletionNewParams {
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage("work as `echo` if possible"),
		openai.UserMessage(message),
	}

	params := openai.ChatCompletionNewParams{
		Model:    model,
		Messages: messages,
	}

	if testMode == ProbeV2ModeTool {
		params.Tools = GetProbeToolsOpenAI()
		params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.Opt("auto"),
		}
	}

	return params
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
func (s *Server) probeProviderWithSDK(ctx context.Context, provider *typ.Provider, model, message string, testMode ProbeMode) (*ProbeV2Data, error) {
	startTime := time.Now()

	clientInterface, err := s.getClientForProvider(provider, model)
	if err != nil {
		return nil, err
	}

	builder := NewSDKProbeBuilder()

	url := provider.APIBase
	if provider.APIStyle == protocol.APIStyleAnthropic {
		url += "/v1/messages"
	} else {
		url += "/chat/completions"
	}

	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		anthropicClient := clientInterface.(*client.AnthropicClient)
		params := builder.buildAnthropicMessageRequest(model, message, testMode)
		resp, err := anthropicClient.MessagesNew(ctx, &params)
		if err != nil {
			return nil, err
		}
		// Convert response to JSON string as content
		respJSON, _ := json.Marshal(resp)
		return &ProbeV2Data{
			Content:    string(respJSON),
			LatencyMs:  time.Since(startTime).Milliseconds(),
			RequestURL: url,
		}, nil

	case protocol.APIStyleOpenAI:
		openaiClient := clientInterface.(*client.OpenAIClient)
		params := builder.buildOpenAIChatRequest(model, message, testMode)
		resp, err := openaiClient.ChatCompletionsNew(ctx, params)
		if err != nil {
			return nil, err
		}
		// Convert response to JSON string as content
		respJSON, _ := json.Marshal(resp)
		return &ProbeV2Data{
			Content:    string(respJSON),
			LatencyMs:  time.Since(startTime).Milliseconds(),
			RequestURL: url,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported API style: %s", provider.APIStyle)
	}
}

// probeProviderStream performs a streaming probe for a provider using SDK
func (s *Server) probeProviderStream(ctx context.Context, provider *typ.Provider, model, message string, testMode ProbeMode) (*ProbeV2Data, error) {
	startTime := time.Now()

	clientInterface, err := s.getClientForProvider(provider, model)
	if err != nil {
		return nil, err
	}

	builder := NewSDKProbeBuilder()

	url := provider.APIBase
	if provider.APIStyle == protocol.APIStyleAnthropic {
		url += "/v1/messages"
	} else {
		url += "/chat/completions"
	}

	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		anthropicClient := clientInterface.(*client.AnthropicClient)
		params := builder.buildAnthropicMessageRequest(model, message, testMode)
		stream := anthropicClient.MessagesNewStreaming(ctx, &params)
		defer stream.Close()

		var chunks []interface{}
		for stream.Next() {
			event := stream.Current()
			// Collect each event as-is
			chunks = append(chunks, event)
		}

		if err := stream.Err(); err != nil {
			return nil, err
		}

		// Convert chunks to JSON string as content
		chunksJSON, _ := json.Marshal(chunks)
		return &ProbeV2Data{
			Content:    string(chunksJSON),
			LatencyMs:  time.Since(startTime).Milliseconds(),
			RequestURL: url,
		}, nil

	case protocol.APIStyleOpenAI:
		openaiClient := clientInterface.(*client.OpenAIClient)
		params := builder.buildOpenAIChatRequest(model, message, testMode)
		stream := openaiClient.ChatCompletionsNewStreaming(ctx, params)
		defer stream.Close()

		var chunks []interface{}
		for stream.Next() {
			chunk := stream.Current()
			// Collect each chunk as-is
			chunks = append(chunks, chunk)
		}

		if err := stream.Err(); err != nil {
			return nil, err
		}

		// Convert chunks to JSON string as content
		chunksJSON, _ := json.Marshal(chunks)
		return &ProbeV2Data{
			Content:    string(chunksJSON),
			LatencyMs:  time.Since(startTime).Milliseconds(),
			RequestURL: url,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported API style: %s", provider.APIStyle)
	}
}
