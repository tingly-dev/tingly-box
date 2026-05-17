package probe

import (
	"context"
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// LightweightService runs the optional "Test Connection" probe used when a
// user adds an API key. It pokes OPTIONS, /models, /chat/completions, and
// /responses and returns a per-endpoint report; results are advisory only
// and do not block onboarding. Independent of *Server.
type LightweightService struct {
	pool *client.ClientPool
}

// NewLightweightService constructs a LightweightService backed by the given client pool.
func NewLightweightService(pool *client.ClientPool) *LightweightService {
	return &LightweightService{pool: pool}
}

// Probe runs every applicable sub-probe for the provider and returns a
// populated LightweightProbeResponseData. Never returns an error — partial
// failure is encoded in the per-endpoint fields and the Valid summary.
func (l *LightweightService) Probe(ctx context.Context, provider *typ.Provider) *LightweightProbeResponseData {
	data := &LightweightProbeResponseData{
		Provider: provider.Name,
		APIBase:  provider.APIBase,
		APIStyle: string(provider.APIStyle),
	}

	optionsResult := l.probeOptionsEndpoint(ctx, provider)
	data.OptionsSuccess = optionsResult.Success
	data.OptionsMessage = optionsResult.Message
	data.OptionsResponseTime = optionsResult.ResponseTime

	modelsResult := l.probeModelsEndpoint(ctx, provider)
	data.ModelsSuccess = modelsResult.Success
	data.ModelsMessage = modelsResult.Message
	data.ModelsResponseTime = modelsResult.ResponseTime
	data.ModelsCount = modelsResult.ModelsCount
	data.Warning = modelsResult.Warning

	if provider.APIStyle == protocol.APIStyleOpenAI {
		chatResult := l.probeChatEndpoint(ctx, provider)
		data.ChatSuccess = chatResult.Success
		data.ChatMessage = chatResult.Message
		data.ChatResponseTime = chatResult.ResponseTime

		responsesResult := l.probeResponsesEndpoint(ctx, provider)
		data.ResponsesSuccess = responsesResult.Success
		data.ResponsesMessage = responsesResult.Message
		data.ResponsesResponseTime = responsesResult.ResponseTime
	}

	data.Valid = data.OptionsSuccess || data.ModelsSuccess || data.ChatSuccess || data.ResponsesSuccess

	if data.Valid {
		successCount := 0
		if data.OptionsSuccess {
			successCount++
		}
		if data.ModelsSuccess {
			successCount++
		}
		if data.ChatSuccess {
			successCount++
		}
		if data.ResponsesSuccess {
			successCount++
		}
		data.Message = fmt.Sprintf("Connection test completed - %d/%d endpoints accessible", successCount, 4)
	} else {
		data.Message = "Connection test failed - unable to reach any provider endpoint"
	}

	return data
}

type endpointReport struct {
	Success      bool
	Message      string
	ResponseTime int64
}

type modelsReport struct {
	Success      bool
	Message      string
	ResponseTime int64
	ModelsCount  int
	Warning      string
}

func (l *LightweightService) probeOptionsEndpoint(ctx context.Context, provider *typ.Provider) endpointReport {
	startTime := time.Now()

	var result client.ProbeResult

	switch provider.APIStyle {
	case protocol.APIStyleOpenAI:
		c := l.pool.GetOpenAIClient(context.Background(), provider, "")
		if c == nil {
			return endpointReport{false, "Failed to create OpenAI client", 0}
		}
		openaiClient, ok := c.(*client.OpenAIClient)
		if !ok {
			return endpointReport{false, "OPTIONS probe not implemented for this client type", 0}
		}
		result = openaiClient.ProbeOptionsEndpoint(ctx)
	case protocol.APIStyleAnthropic:
		c := l.pool.GetAnthropicClient(context.Background(), provider, "")
		if c == nil {
			return endpointReport{false, "Failed to create Anthropic client", 0}
		}
		anthropicClient, ok := c.(*client.AnthropicClient)
		if !ok {
			return endpointReport{false, "OPTIONS probe not implemented for this client type", 0}
		}
		result = anthropicClient.ProbeOptionsEndpoint(ctx)
	case protocol.APIStyleGoogle:
		c := l.pool.GetGoogleClient(context.Background(), provider, "")
		if c == nil {
			return endpointReport{false, "Failed to create Google client", 0}
		}
		result = c.ProbeOptionsEndpoint(ctx)
	default:
		return endpointReport{false, fmt.Sprintf("Unsupported API style: %s", provider.APIStyle), 0}
	}

	responseTime := time.Since(startTime).Milliseconds()
	if result.Success {
		return endpointReport{true, "OPTIONS request successful", responseTime}
	}
	return endpointReport{false, fmt.Sprintf("OPTIONS failed: %s", result.ErrorMessage), responseTime}
}

func (l *LightweightService) probeModelsEndpoint(ctx context.Context, provider *typ.Provider) modelsReport {
	startTime := time.Now()

	var lister client.ModelLister

	switch provider.APIStyle {
	case protocol.APIStyleOpenAI:
		c := l.pool.GetOpenAIClient(context.Background(), provider, "")
		if c == nil {
			return modelsReport{false, "Failed to create OpenAI client", 0, 0, ""}
		}
		lister = c
	case protocol.APIStyleAnthropic:
		c := l.pool.GetAnthropicClient(context.Background(), provider, "")
		if c == nil {
			return modelsReport{false, "Failed to create Anthropic client", 0, 0, ""}
		}
		lister = c
	case protocol.APIStyleGoogle:
		c := l.pool.GetGoogleClient(context.Background(), provider, "")
		if c == nil {
			return modelsReport{false, "Failed to create Google client", 0, 0, ""}
		}
		lister = c
	default:
		return modelsReport{false, fmt.Sprintf("Unsupported API style: %s", provider.APIStyle), 0, 0, ""}
	}

	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	models, err := lister.ListModels(probeCtx)
	responseTime := time.Since(startTime).Milliseconds()

	if client.IsModelsEndpointNotSupported(err) {
		return modelsReport{
			false,
			"Models endpoint not supported for this provider type",
			responseTime,
			0,
			"This provider does not support the models list endpoint (e.g., OAuth-based providers)",
		}
	}

	if err != nil {
		return modelsReport{false, fmt.Sprintf("Models endpoint failed: %v", err), responseTime, 0, ""}
	}

	if len(models) == 0 {
		return modelsReport{false, "Models endpoint returned no models", responseTime, 0, ""}
	}

	return modelsReport{
		true,
		fmt.Sprintf("Models endpoint accessible - %d models found", len(models)),
		responseTime,
		len(models),
		"",
	}
}

func (l *LightweightService) probeChatEndpoint(ctx context.Context, provider *typ.Provider) endpointReport {
	startTime := time.Now()

	c := l.pool.GetOpenAIClient(context.Background(), provider, "")
	if c == nil {
		return endpointReport{false, "Failed to create OpenAI client", 0}
	}

	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	result, err := c.ProbeChatEndpoint(probeCtx, "gpt-3.5-turbo", client.ProbeEndpointOptions{
		Message: "Hi",
		Stream:  false,
		Mode:    client.ProbeModeSimple,
	})
	responseTime := time.Since(startTime).Milliseconds()

	if err != nil {
		return endpointReport{false, fmt.Sprintf("Chat endpoint failed: %v", err), responseTime}
	}
	if result != nil && result.Content != "" {
		return endpointReport{true, "Chat endpoint accessible", responseTime}
	}
	return endpointReport{false, "Chat endpoint returned no content", responseTime}
}

func (l *LightweightService) probeResponsesEndpoint(ctx context.Context, provider *typ.Provider) endpointReport {
	startTime := time.Now()

	c := l.pool.GetOpenAIClient(context.Background(), provider, "")
	if c == nil {
		return endpointReport{false, "Failed to create OpenAI client", 0}
	}

	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	result, err := c.ProbeResponsesEndpoint(probeCtx, "gpt-4o", client.ProbeEndpointOptions{
		Message: "Hi",
		Stream:  false,
		Mode:    client.ProbeModeSimple,
	})
	responseTime := time.Since(startTime).Milliseconds()

	if err != nil {
		return endpointReport{false, fmt.Sprintf("Responses endpoint failed: %v", err), responseTime}
	}
	if result != nil && result.Content != "" {
		return endpointReport{true, "Responses API endpoint accessible", responseTime}
	}
	return endpointReport{false, "Responses endpoint returned no content", responseTime}
}
