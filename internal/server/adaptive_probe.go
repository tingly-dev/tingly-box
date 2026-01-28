package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

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
		return ap.probeOpenAIChatEndpoint(ctx, provider, modelID, startTime)
	case protocol.APIStyleAnthropic:
		return ap.probeAnthropicChatEndpoint(ctx, provider, modelID, startTime)
	default:
		return EndpointStatus{
			Available:    false,
			ErrorMessage: fmt.Sprintf("Unsupported API style: %s", provider.APIStyle),
			LastChecked:  time.Now(),
		}
	}
}

// probeOpenAIChatEndpoint probes OpenAI-style chat completions endpoint
func (ap *AdaptiveProbe) probeOpenAIChatEndpoint(ctx context.Context, provider *typ.Provider, modelID string, startTime time.Time) EndpointStatus {
	apiBase := strings.TrimSuffix(provider.APIBase, "/")
	if !strings.Contains(apiBase, "/v1") {
		apiBase = apiBase + "/v1"
	}

	chatURL := apiBase + "/chat/completions"

	requestBody := map[string]interface{}{
		"model": modelID,
		"messages": []map[string]string{
			{"role": "user", "content": "Hi"},
		},
		"max_tokens": 5,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return EndpointStatus{
			Available:    false,
			ErrorMessage: fmt.Sprintf("Failed to marshal request: %v", err),
			LastChecked:  time.Now(),
		}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", chatURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return EndpointStatus{
			Available:    false,
			ErrorMessage: fmt.Sprintf("Failed to create request: %v", err),
			LastChecked:  time.Now(),
		}
	}

	req.Header.Set("Authorization", "Bearer "+provider.Token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: DefaultProbeTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return EndpointStatus{
			Available:    false,
			ErrorMessage: fmt.Sprintf("Chat request failed: %v", err),
			LastChecked:  time.Now(),
		}
	}
	defer resp.Body.Close()

	latency := int(time.Since(startTime).Milliseconds())

	// Consider 200 or 429 (rate limit) as available
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusTooManyRequests {
		return EndpointStatus{
			Available:    true,
			LatencyMs:    latency,
			ErrorMessage: "",
			LastChecked:  time.Now(),
		}
	}

	// Read error message from response body
	bodyBytes, _ = readResponseBody(resp.Body)
	errorMsg := fmt.Sprintf("Chat endpoint failed with status %d: %s", resp.StatusCode, string(bodyBytes))

	return EndpointStatus{
		Available:    false,
		LatencyMs:    latency,
		ErrorMessage: errorMsg,
		LastChecked:  time.Now(),
	}
}

// probeAnthropicChatEndpoint probes Anthropic-style messages endpoint
func (ap *AdaptiveProbe) probeAnthropicChatEndpoint(ctx context.Context, provider *typ.Provider, modelID string, startTime time.Time) EndpointStatus {
	apiBase := strings.TrimSuffix(provider.APIBase, "/")
	if !strings.Contains(apiBase, "/v1") {
		apiBase = apiBase + "/v1"
	}

	messagesURL := apiBase + "/messages"

	requestBody := map[string]interface{}{
		"model":      modelID,
		"max_tokens": 5,
		"messages": []map[string]string{
			{"role": "user", "content": "Hi"},
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return EndpointStatus{
			Available:    false,
			ErrorMessage: fmt.Sprintf("Failed to marshal request: %v", err),
			LastChecked:  time.Now(),
		}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", messagesURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return EndpointStatus{
			Available:    false,
			ErrorMessage: fmt.Sprintf("Failed to create request: %v", err),
			LastChecked:  time.Now(),
		}
	}

	req.Header.Set("x-api-key", provider.Token)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: DefaultProbeTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return EndpointStatus{
			Available:    false,
			ErrorMessage: fmt.Sprintf("Messages request failed: %v", err),
			LastChecked:  time.Now(),
		}
	}
	defer resp.Body.Close()

	latency := int(time.Since(startTime).Milliseconds())

	// Consider 200 or 429 (rate limit) as available
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusTooManyRequests {
		return EndpointStatus{
			Available:    true,
			LatencyMs:    latency,
			ErrorMessage: "",
			LastChecked:  time.Now(),
		}
	}

	// Read error message from response body
	bodyBytes, _ = readResponseBody(resp.Body)
	errorMsg := fmt.Sprintf("Messages endpoint failed with status %d: %s", resp.StatusCode, string(bodyBytes))

	return EndpointStatus{
		Available:    false,
		LatencyMs:    latency,
		ErrorMessage: errorMsg,
		LastChecked:  time.Now(),
	}
}

// probeResponsesEndpoint probes the Responses API endpoint for a model
func (ap *AdaptiveProbe) probeResponsesEndpoint(ctx context.Context, provider *typ.Provider, modelID string) EndpointStatus {
	startTime := time.Now()

	// Get OpenAI client from pool
	wrapper := ap.server.clientPool.GetOpenAIClient(provider, "")
	if wrapper == nil {
		return EndpointStatus{
			Available:    false,
			ErrorMessage: "Failed to get OpenAI client",
			LastChecked:  time.Now(),
		}
	}

	// Create minimal Responses API request using raw JSON approach
	// This avoids the complex type issues with the SDK
	params := responses.ResponseNewParams{
		Input: responses.ResponseNewParamsInputUnion{OfString: param.NewOpt("Hi")},
	}

	// Set model via raw JSON - marshal to JSON, set model, unmarshal back
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return EndpointStatus{
			Available:    false,
			ErrorMessage: fmt.Sprintf("Failed to marshal params: %v", err),
			LastChecked:  time.Now(),
		}
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(paramsJSON, &raw); err != nil {
		return EndpointStatus{
			Available:    false,
			ErrorMessage: fmt.Sprintf("Failed to unmarshal params: %v", err),
			LastChecked:  time.Now(),
		}
	}

	raw["model"] = modelID

	modifiedJSON, err := json.Marshal(raw)
	if err != nil {
		return EndpointStatus{
			Available:    false,
			ErrorMessage: fmt.Sprintf("Failed to marshal modified params: %v", err),
			LastChecked:  time.Now(),
		}
	}

	if err := json.Unmarshal(modifiedJSON, &params); err != nil {
		return EndpointStatus{
			Available:    false,
			ErrorMessage: fmt.Sprintf("Failed to unmarshal modified params: %v", err),
			LastChecked:  time.Now(),
		}
	}

	// Make the request
	resp, err := wrapper.Client().Responses.New(ctx, params)
	latency := int(time.Since(startTime).Milliseconds())

	if err != nil {
		return EndpointStatus{
			Available:    false,
			LatencyMs:    latency,
			ErrorMessage: fmt.Sprintf("Responses API request failed: %v", err),
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
		ErrorMessage: "Responses API returned invalid response",
		LastChecked:  time.Now(),
	}
}

// determinePreferredEndpoint determines which endpoint to prefer based on availability
func (ap *AdaptiveProbe) determinePreferredEndpoint(chat, responses *EndpointStatus) string {
	// Responses API is preferred when available
	if responses.Available {
		return string(db.EndpointTypeResponses)
	}
	if chat.Available {
		return string(db.EndpointTypeChat)
	}
	return "" // Neither available
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
func (ap *AdaptiveProbe) GetModelCapability(providerUUID, modelID string) (*ModelEndpointCapability, error) {
	// Check cache first
	if ap.server.probeCache != nil {
		if cached := ap.server.probeCache.Get(providerUUID, modelID); cached != nil {
			return cached, nil
		}
	}

	// Check database
	if ap.server.capabilityStore != nil {
		if dbCapability, found := ap.server.capabilityStore.GetModelCapability(providerUUID, modelID); found {
			// Check if database record is stale
			if ap.server.probeCache != nil && time.Since(dbCapability.LastVerified) > ap.server.probeCache.ttl {
				// Database record is stale, trigger async probe refresh
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), DefaultProbeTimeout)
					defer cancel()
					ap.ProbeModelEndpoints(ctx, ModelProbeRequest{
						ProviderUUID: providerUUID,
						ModelID:      modelID,
					})
				}()

				// Return nil to trigger default behavior, or return stale data with expectation of refresh
				// For now, return stale data but trigger refresh
			}

			// Convert DB capability to internal capability type
			capability := &ModelEndpointCapability{
				ProviderUUID:       dbCapability.ProviderUUID,
				ModelID:            dbCapability.ModelID,
				SupportsChat:       dbCapability.SupportsChat,
				ChatLatencyMs:      dbCapability.ChatLatencyMs,
				ChatError:          dbCapability.ChatError,
				SupportsResponses:  dbCapability.SupportsResponses,
				ResponsesLatencyMs: dbCapability.ResponsesLatencyMs,
				ResponsesError:     dbCapability.ResponsesError,
				PreferredEndpoint:  dbCapability.PreferredEndpoint,
				LastVerified:       dbCapability.LastVerified,
			}

			// Also cache it (even if stale, will be refreshed soon)
			if ap.server.probeCache != nil {
				ap.server.probeCache.Set(providerUUID, modelID, capability)
			}
			return capability, nil
		}
	}

	return nil, fmt.Errorf("model capability not found for provider %s, model %s", providerUUID, modelID)
}

// GetPreferredEndpoint returns the preferred endpoint for a model
func (ap *AdaptiveProbe) GetPreferredEndpoint(provider *typ.Provider, modelID string) string {
	capability, err := ap.GetModelCapability(provider.UUID, modelID)
	if err != nil {
		// Trigger async probe refresh
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), DefaultProbeTimeout)
			defer cancel()
			ap.ProbeModelEndpoints(ctx, ModelProbeRequest{
				ProviderUUID: provider.UUID,
				ModelID:      modelID,
			})
		}()

		// Default to chat for unknown models
		return string(db.EndpointTypeChat)
	}

	if capability.PreferredEndpoint == string(db.EndpointTypeResponses) {
		return string(db.EndpointTypeResponses)
	}
	return string(db.EndpointTypeChat)
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
