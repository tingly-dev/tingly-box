package server

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const (
	// DefaultProbeTimeout is the default timeout for each endpoint probe
	DefaultProbeTimeout = 10 * time.Second
	// DefaultCacheTTL is the default time-to-live for cached probe results
	DefaultCacheTTL = 24 * time.Hour
)

// AdaptiveProbe handles concurrent endpoint probing for model capabilities
type AdaptiveProbe struct {
	server *Server
}

// NewAdaptiveProbe creates a new adaptive probe instance
func NewAdaptiveProbe(s *Server) *AdaptiveProbe {
	return &AdaptiveProbe{server: s}
}

// ProbeModelEndpoints probes both chat and responses endpoints concurrently for a model
func (ap *AdaptiveProbe) ProbeModelEndpoints(ctx context.Context, req ModelProbeRequest) (*ProbeResult, error) {
	// Step 1: Get provider
	provider, err := ap.server.config.GetProviderByUUID(req.ProviderUUID)
	if err != nil || provider == nil {
		return nil, fmt.Errorf("provider not found: %s", req.ProviderUUID)
	}

	// Step 2: Check cache first (unless force refresh)
	if !req.ForceRefresh && ap.server.probeCache != nil {
		if cached := ap.server.probeCache.Get(req.ProviderUUID, req.ModelID); cached != nil {
			// Convert cached capability to probe result
			logrus.Debugf("Prefer cache hit for provider %s, %s", req.ProviderUUID, cached.PreferredEndpoint)
			return ap.cachedCapabilityToResult(cached), nil
		}
	}

	// Step 3: Run probes concurrently
	var wg sync.WaitGroup
	var chatStatus, responsesStatus EndpointStatus

	// Create context with timeout for both probes
	probeCtx, cancel := context.WithTimeout(ctx, DefaultProbeTimeout)
	defer cancel()

	// Probe chat endpoint
	wg.Add(1)
	go func() {
		defer wg.Done()
		chatStatus = ap.probeChatEndpoint(probeCtx, provider, req.ModelID)
	}()

	// Probe responses endpoint (only for OpenAI-style providers)
	if provider.APIStyle == protocol.APIStyleOpenAI {
		wg.Add(1)
		go func() {
			defer wg.Done()
			responsesStatus = ap.probeResponsesEndpoint(probeCtx, provider, req.ModelID)
		}()
	} else {
		// Mark responses as unavailable for non-OpenAI providers
		responsesStatus = EndpointStatus{
			Available:    false,
			ErrorMessage: "Responses API is only supported by OpenAI-style providers",
			LastChecked:  time.Now(),
		}
	}

	// Wait for all probes to complete
	wg.Wait()

	// Step 4: Determine preferred endpoint
	preferred := ap.determinePreferredEndpoint(&chatStatus, &responsesStatus)

	result := &ProbeResult{
		ProviderUUID:      req.ProviderUUID,
		ModelID:           req.ModelID,
		ChatEndpoint:      chatStatus,
		ResponsesEndpoint: responsesStatus,
		PreferredEndpoint: preferred,
		LastUpdated:       time.Now(),
	}

	// Step 5: Cache results
	if ap.server.probeCache != nil {
		ap.server.probeCache.SetFromProbeResult(result)
	}

	// Step 6: Persist to database
	if ap.server.capabilityStore != nil {
		ap.persistResult(result)
	}

	return result, nil
}

// probeChatEndpoint probes the chat completions endpoint for a model
func (ap *AdaptiveProbe) probeChatEndpoint(ctx context.Context, provider *typ.Provider, modelID string) EndpointStatus {
	startTime := time.Now()

	switch provider.APIStyle {
	case protocol.APIStyleOpenAI:
		return ap.probeOpenAIChatEndpointWithSDK(ctx, provider, modelID, startTime)
	case protocol.APIStyleAnthropic:
		return ap.probeAnthropicChatEndpointWithSDK(ctx, provider, modelID, startTime)
	default:
		return EndpointStatus{
			Available:    false,
			ErrorMessage: fmt.Sprintf("Unsupported API style: %s", provider.APIStyle),
			LastChecked:  time.Now(),
		}
	}
}

// probeOpenAIChatEndpointWithSDK probes OpenAI-style chat completions endpoint using SDK.
// Tests streaming capability only.
func (ap *AdaptiveProbe) probeOpenAIChatEndpointWithSDK(ctx context.Context, provider *typ.Provider, modelID string, startTime time.Time) EndpointStatus {
	// Get OpenAI client from pool
	wrapper := ap.server.clientPool.GetOpenAIClient(context.Background(), provider, "")
	if wrapper == nil {
		return EndpointStatus{
			Available:      false,
			SupportsStream: false,
			ErrorMessage:   "Failed to get OpenAI client",
			LastChecked:    time.Now(),
		}
	}

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage("Hi"),
	}

	// Create a context with timeout for the probe
	probeCtx, cancel := context.WithTimeout(ctx, DefaultProbeTimeout)
	defer cancel()

	// Test streaming capability
	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(modelID),
		Messages: messages,
	}
	stream := wrapper.Client().Chat.Completions.NewStreaming(probeCtx, params)
	defer stream.Close()

	streamWorks := false
	for stream.Next() {
		streamWorks = true
		break // Got at least one chunk, streaming works
	}

	latency := int(time.Since(startTime).Milliseconds())

	if err := stream.Err(); err != nil {
		return EndpointStatus{
			Available:      false,
			SupportsStream: false,
			LatencyMs:      latency,
			ErrorMessage:   fmt.Sprintf("Chat streaming failed: %v", err),
			LastChecked:    time.Now(),
		}
	}

	if streamWorks {
		return EndpointStatus{
			Available:      true,
			SupportsStream: true,
			LatencyMs:      latency,
			ErrorMessage:   "",
			LastChecked:    time.Now(),
		}
	}

	return EndpointStatus{
		Available:      false,
		SupportsStream: false,
		LatencyMs:      latency,
		ErrorMessage:   "Chat streaming returned no data",
		LastChecked:    time.Now(),
	}
}

// probeAnthropicChatEndpointWithSDK probes Anthropic-style messages endpoint using SDK
func (ap *AdaptiveProbe) probeAnthropicChatEndpointWithSDK(ctx context.Context, provider *typ.Provider, modelID string, startTime time.Time) EndpointStatus {
	// Get Anthropic client from pool
	wrapper := ap.server.clientPool.GetAnthropicClient(context.Background(), provider, "")
	if wrapper == nil {
		return EndpointStatus{
			Available:    false,
			ErrorMessage: "Failed to get Anthropic client",
			LastChecked:  time.Now(),
		}
	}

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("Hi")),
	}

	params := &anthropic.MessageNewParams{
		Model:     anthropic.Model(modelID),
		MaxTokens: 5,
		Messages:  messages,
	}

	// Create a context with timeout for the probe
	probeCtx, cancel := context.WithTimeout(ctx, DefaultProbeTimeout)
	defer cancel()

	resp, err := wrapper.MessagesNew(probeCtx, params)
	latency := int(time.Since(startTime).Milliseconds())

	if err != nil {
		return EndpointStatus{
			Available:    false,
			LatencyMs:    latency,
			ErrorMessage: fmt.Sprintf("Messages request failed: %v", err),
			LastChecked:  time.Now(),
		}
	}

	// Check if response is valid
	if resp != nil && resp.ID != "" {
		return EndpointStatus{
			Available:    true,
			LatencyMs:    latency,
			ErrorMessage: "",
			LastChecked:  time.Now(),
		}
	}

	return EndpointStatus{
		Available:    false,
		LatencyMs:    latency,
		ErrorMessage: "Messages endpoint returned invalid response",
		LastChecked:  time.Now(),
	}
}

// probeOpenAIChatEndpoint probes OpenAI-style chat completions endpoint (deprecated, use SDK version)
// Kept for backwards compatibility only
func (ap *AdaptiveProbe) probeOpenAIChatEndpoint(ctx context.Context, provider *typ.Provider, modelID string, startTime time.Time) EndpointStatus {
	return ap.probeOpenAIChatEndpointWithSDK(ctx, provider, modelID, startTime)
}

// probeAnthropicChatEndpoint probes Anthropic-style messages endpoint (deprecated, use SDK version)
// Kept for backwards compatibility only
func (ap *AdaptiveProbe) probeAnthropicChatEndpoint(ctx context.Context, provider *typ.Provider, modelID string, startTime time.Time) EndpointStatus {
	return ap.probeAnthropicChatEndpointWithSDK(ctx, provider, modelID, startTime)
}

// probeResponsesEndpoint probes the Responses API endpoint for a model.
// Tests streaming capability only.
func (ap *AdaptiveProbe) probeResponsesEndpoint(ctx context.Context, provider *typ.Provider, modelID string) EndpointStatus {
	startTime := time.Now()

	// Get OpenAI client from pool
	wrapper := ap.server.clientPool.GetOpenAIClient(context.Background(), provider, modelID)
	if wrapper == nil {
		return EndpointStatus{
			Available:      false,
			SupportsStream: false,
			ErrorMessage:   "Failed to get OpenAI client",
			LastChecked:    time.Now(),
		}
	}

	// Create minimal Responses API request
	params := responses.ResponseNewParams{
		Model: modelID,
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: responses.ResponseInputParam{
				{
					OfMessage: &responses.EasyInputMessageParam{
						Role: responses.EasyInputMessageRoleUser,
						Content: responses.EasyInputMessageContentUnionParam{
							OfString: param.NewOpt("hi"),
						},
					},
				},
			},
		},
	}

	// Test streaming capability
	stream := wrapper.Client().Responses.NewStreaming(ctx, params)

	streamWorks := false
	for stream.Next() {
		streamWorks = true
		break // Got at least one chunk, streaming works
	}

	latency := int(time.Since(startTime).Milliseconds())

	if err := stream.Err(); err != nil {
		return EndpointStatus{
			Available:      false,
			SupportsStream: false,
			LatencyMs:      latency,
			ErrorMessage:   fmt.Sprintf("Responses streaming failed: %v", err),
			LastChecked:    time.Now(),
		}
	}

	if streamWorks {
		return EndpointStatus{
			Available:      true,
			SupportsStream: true,
			LatencyMs:      latency,
			ErrorMessage:   "",
			LastChecked:    time.Now(),
		}
	}

	return EndpointStatus{
		Available:      false,
		SupportsStream: false,
		LatencyMs:      latency,
		ErrorMessage:   "Responses streaming returned no data",
		LastChecked:    time.Now(),
	}
}

// determinePreferredEndpoint determines which endpoint to prefer based on availability and streaming support.
// Priority:
// 1. Chat API with streaming support (most stable and widely supported)
// 2. Responses API with streaming support (for specialized models like Codex)
// 3. Chat API without streaming (fallback for compatibility)
// 4. Responses API without streaming (last resort)
func (ap *AdaptiveProbe) determinePreferredEndpoint(chat, responses *EndpointStatus) string {
	// Priority 1: Chat API with streaming (most stable option)
	if chat.Available && chat.SupportsStream {
		return string(db.EndpointTypeChat)
	}

	// Priority 2: Responses API with streaming (only when Chat doesn't support streaming)
	if responses.Available && responses.SupportsStream {
		return string(db.EndpointTypeResponses)
	}

	// Priority 3: Chat API without streaming (better compatibility than Responses)
	if chat.Available {
		return string(db.EndpointTypeChat)
	}

	// Priority 4: Responses API without streaming (last resort)
	if responses.Available {
		return string(db.EndpointTypeResponses)
	}

	return string(db.EndpointTypeChat) // Neither available, use chat as default
}

// persistResult persists probe result to database
func (ap *AdaptiveProbe) persistResult(result *ProbeResult) {
	// Save chat endpoint capability
	err := ap.server.capabilityStore.SaveCapability(
		result.ProviderUUID,
		result.ModelID,
		db.EndpointTypeChat,
		result.ChatEndpoint.Available,
		result.ChatEndpoint.LatencyMs,
		result.ChatEndpoint.ErrorMessage,
	)
	if err != nil {
		logrus.Warnf("Failed to save chat capability: %v", err)
	}

	// Save responses endpoint capability
	err = ap.server.capabilityStore.SaveCapability(
		result.ProviderUUID,
		result.ModelID,
		db.EndpointTypeResponses,
		result.ResponsesEndpoint.Available,
		result.ResponsesEndpoint.LatencyMs,
		result.ResponsesEndpoint.ErrorMessage,
	)
	if err != nil {
		logrus.Warnf("Failed to save responses capability: %v", err)
	}
}

// cachedCapabilityToResult converts cached capability to probe result
func (ap *AdaptiveProbe) cachedCapabilityToResult(capability *ModelEndpointCapability) *ProbeResult {
	return &ProbeResult{
		ProviderUUID: capability.ProviderUUID,
		ModelID:      capability.ModelID,
		ChatEndpoint: EndpointStatus{
			Available:    capability.SupportsChat,
			LatencyMs:    capability.ChatLatencyMs,
			ErrorMessage: capability.ChatError,
			LastChecked:  capability.LastVerified,
		},
		ResponsesEndpoint: EndpointStatus{
			Available:    capability.SupportsResponses,
			LatencyMs:    capability.ResponsesLatencyMs,
			ErrorMessage: capability.ResponsesError,
			LastChecked:  capability.LastVerified,
		},
		PreferredEndpoint: capability.PreferredEndpoint,
		LastUpdated:       capability.LastVerified,
	}
}

// GetModelCapability retrieves cached capability for a model, or triggers a probe if not cached
// IMPORTANT: This no longer reads PreferredEndpoint from database - it only reads the raw
// capability status (SupportsChat, SupportsResponses) and recalculates PreferredEndpoint
// using the current determinePreferredEndpoint logic. This ensures that behavior changes
// take effect immediately upon restart, without stale database data.
func (ap *AdaptiveProbe) GetModelCapability(providerUUID, modelID string) (*ModelEndpointCapability, error) {
	// Check cache first
	if ap.server.probeCache != nil {
		if cached := ap.server.probeCache.Get(providerUUID, modelID); cached != nil {
			return cached, nil
		}
	}

	// Check database for raw capability status (but NOT PreferredEndpoint)
	if ap.server.capabilityStore != nil {
		if dbCapability, found := ap.server.capabilityStore.GetModelCapability(providerUUID, modelID); found {
			// Reconstruct endpoint status from database
			chatStatus := EndpointStatus{
				Available:    dbCapability.SupportsChat,
				LatencyMs:    dbCapability.ChatLatencyMs,
				ErrorMessage: dbCapability.ChatError,
				LastChecked:  dbCapability.LastVerified,
			}
			responsesStatus := EndpointStatus{
				Available:    dbCapability.SupportsResponses,
				LatencyMs:    dbCapability.ResponsesLatencyMs,
				ErrorMessage: dbCapability.ResponsesError,
				LastChecked:  dbCapability.LastVerified,
			}

			// Recalculate PreferredEndpoint using current logic (NOT from database)
			// This ensures behavior changes take effect immediately
			preferredEndpoint := ap.determinePreferredEndpoint(&chatStatus, &responsesStatus)

			capability := &ModelEndpointCapability{
				ProviderUUID:       dbCapability.ProviderUUID,
				ModelID:            dbCapability.ModelID,
				SupportsChat:       dbCapability.SupportsChat,
				ChatLatencyMs:      dbCapability.ChatLatencyMs,
				ChatError:          dbCapability.ChatError,
				SupportsResponses:  dbCapability.SupportsResponses,
				ResponsesLatencyMs: dbCapability.ResponsesLatencyMs,
				ResponsesError:     dbCapability.ResponsesError,
				PreferredEndpoint:  preferredEndpoint, // Recalculated, not from DB
				LastVerified:       dbCapability.LastVerified,
			}

			// Cache the recalculated capability
			if ap.server.probeCache != nil {
				ap.server.probeCache.Set(providerUUID, modelID, capability)
			}

			// If data is stale, trigger async probe refresh
			if ap.server.probeCache != nil && time.Since(dbCapability.LastVerified) > ap.server.probeCache.ttl {
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), DefaultProbeTimeout)
					defer cancel()
					ap.ProbeModelEndpoints(ctx, ModelProbeRequest{
						ProviderUUID: providerUUID,
						ModelID:      modelID,
					})
				}()
			}

			return capability, nil
		}
	}

	return nil, fmt.Errorf("model capability not found for provider %s, model %s", providerUUID, modelID)
}

// GetPreferredEndpoint returns the preferred endpoint for a model.
//
// Resolution order:
// 1. Check memory cache (fast, includes PreferredEndpoint)
// 2. Check database for raw state, recalculate PreferredEndpoint
// 3. Trigger synchronous probe if no cached data
// 4. Default to "chat" if probe fails (safe fallback)
//
// NOTE: The PreferredEndpoint is ALWAYS calculated by determinePreferredEndpoint(),
// never read from database. This ensures behavior changes take effect immediately.
func (ap *AdaptiveProbe) GetPreferredEndpoint(provider *typ.Provider, modelID string) string {
	// For now, all models with "codex" in their name (case insensitive) prefer completions
	// In the future, this can be extended to support more models or be configured per-model
	if strings.Contains(strings.ToLower(modelID), "codex") {
		return string(db.EndpointTypeResponses)
	}
	if strings.Contains(strings.ToLower(provider.APIBase), "chatgpt.com") {
		return string(db.EndpointTypeResponses)
	}

	capability, err := ap.GetModelCapability(provider.UUID, modelID)
	if err != nil || capability.PreferredEndpoint == "" {
		// No cached data - trigger synchronous probe
		// This is the cold start path
		ctx, cancel := context.WithTimeout(context.Background(), DefaultProbeTimeout)
		defer cancel()
		res, err := ap.ProbeModelEndpoints(ctx, ModelProbeRequest{
			ProviderUUID: provider.UUID,
			ModelID:      modelID,
		})

		if err != nil {
			logrus.Warnf("Failed to get model capability for %s/%s: %v", provider.Name, modelID, err)
			// Safe fallback: default to chat endpoint
			return string(db.EndpointTypeChat)
		}
		return res.PreferredEndpoint
	}

	// Return the cached preferred endpoint
	return capability.PreferredEndpoint
}

// InvalidateProviderCache invalidates all cached capabilities for a provider
func (ap *AdaptiveProbe) InvalidateProviderCache(providerUUID string) {
	if ap.server.probeCache != nil {
		ap.server.probeCache.InvalidateProvider(providerUUID)
	}
}

// ProbeProviderModels probes all models for a provider concurrently
func (ap *AdaptiveProbe) ProbeProviderModels(ctx context.Context, provider *typ.Provider, models []string) map[string]*ProbeResult {
	results := make(map[string]*ProbeResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Limit concurrency to avoid overwhelming the provider
	semaphore := make(chan struct{}, 5)

	for _, model := range models {
		wg.Add(1)
		go func(modelID string) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			req := ModelProbeRequest{
				ProviderUUID: provider.UUID,
				ModelID:      modelID,
			}

			result, err := ap.ProbeModelEndpoints(ctx, req)
			if err == nil {
				mu.Lock()
				results[modelID] = result
				mu.Unlock()
			}
		}(model)
	}

	wg.Wait()
	return results
}

// readResponseBody reads response body with error handling
func readResponseBody(body io.ReadCloser) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	defer body.Close()
	return io.ReadAll(body)
}
