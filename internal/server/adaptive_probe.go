package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/probe"
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
func (ap *AdaptiveProbe) ProbeModelEndpoints(ctx context.Context, req probe.ModelProbeRequest) (*probe.ProbeResult, error) {
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

	// Step 3: Run probes concurrently.
	result := ap.runEndpointProbes(ctx, provider, req)

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

func (ap *AdaptiveProbe) runEndpointProbes(ctx context.Context, provider *typ.Provider, req probe.ModelProbeRequest) *probe.ProbeResult {
	var wg sync.WaitGroup
	var chatStatus, responsesStatus probe.EndpointStatus

	probeCtx, cancel := context.WithTimeout(ctx, DefaultProbeTimeout)
	defer cancel()

	if !isCodexProvider(provider) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			chatStatus = ap.probeChatEndpoint(probeCtx, provider, req.ModelID)
		}()
	} else {
		chatStatus = probe.EndpointStatus{Available: false, ErrorMessage: "Codex provider does not support Chat Completions API", LastChecked: time.Now()}
	}

	if provider.APIStyle == protocol.APIStyleOpenAI {
		wg.Add(1)
		go func() {
			defer wg.Done()
			responsesStatus = ap.probeOpenAIResponsesEndpoint(probeCtx, provider, req.ModelID)
		}()
	} else {
		responsesStatus = probe.EndpointStatus{Available: false, ErrorMessage: "Responses API is only supported by OpenAI-style providers", LastChecked: time.Now()}
	}

	wg.Wait()
	preferred := ap.determinePreferredEndpoint(&chatStatus, &responsesStatus)
	return &probe.ProbeResult{
		ProviderUUID:      req.ProviderUUID,
		ModelID:           req.ModelID,
		ChatEndpoint:      chatStatus,
		ResponsesEndpoint: responsesStatus,
		PreferredEndpoint: preferred,
		LastUpdated:       time.Now(),
	}
}

// probeChatEndpoint probes the chat completions endpoint for a model using Prober interface
func (ap *AdaptiveProbe) probeChatEndpoint(ctx context.Context, provider *typ.Provider, modelID string) probe.EndpointStatus {
	startTime := time.Now()

	// Get prober from client pool
	var prober client.Prober
	switch provider.APIStyle {
	case protocol.APIStyleOpenAI:
		wrapper := ap.server.clientPool.GetOpenAIClient(context.Background(), provider, "")
		if wrapper == nil {
			return probe.EndpointStatus{
				Available:    false,
				ErrorMessage: "Failed to get OpenAI client",
				LastChecked:  time.Now(),
			}
		}
		result, err := wrapper.ProbeChatEndpoint(ctx, modelID, client.ProbeEndpointOptions{Message: "Hi", Stream: true, Mode: client.ProbeModeStreaming})
		latency := int(time.Since(startTime).Milliseconds())
		if err != nil {
			return probe.EndpointStatus{Available: false, LatencyMs: latency, ErrorMessage: fmt.Sprintf("Chat probe failed: %v", err), LastChecked: time.Now()}
		}
		if result != nil && result.Content != "" {
			return probe.EndpointStatus{Available: true, SupportsStream: true, LatencyMs: latency, LastChecked: time.Now()}
		}
		return probe.EndpointStatus{Available: false, LatencyMs: latency, ErrorMessage: "Chat probe returned no content", LastChecked: time.Now()}
	case protocol.APIStyleAnthropic:
		wrapper := ap.server.clientPool.GetAnthropicClient(context.Background(), provider, "")
		if wrapper == nil {
			return probe.EndpointStatus{
				Available:    false,
				ErrorMessage: "Failed to get Anthropic client",
				LastChecked:  time.Now(),
			}
		}
		prober = wrapper
	default:
		return probe.EndpointStatus{
			Available:    false,
			ErrorMessage: fmt.Sprintf("Unsupported API style: %s", provider.APIStyle),
			LastChecked:  time.Now(),
		}
	}

	// Create a context with timeout for the probe
	probeCtx, cancel := context.WithTimeout(ctx, DefaultProbeTimeout)
	defer cancel()

	// Use ProbeStream with simple mode to test the endpoint
	result, err := prober.ProbeStream(probeCtx, modelID, "Hi", client.ProbeModeSimple)
	latency := int(time.Since(startTime).Milliseconds())

	if err != nil {
		return probe.EndpointStatus{
			Available:    false,
			LatencyMs:    latency,
			ErrorMessage: fmt.Sprintf("Chat probe failed: %v", err),
			LastChecked:  time.Now(),
		}
	}

	if result != nil && result.Content != "" {
		return probe.EndpointStatus{
			Available:      true,
			SupportsStream: true,
			LatencyMs:      latency,
			ErrorMessage:   "",
			LastChecked:    time.Now(),
		}
	}

	return probe.EndpointStatus{
		Available:    false,
		LatencyMs:    latency,
		ErrorMessage: "Chat probe returned no content",
		LastChecked:  time.Now(),
	}
}

// probeOpenAIResponsesEndpoint probes the Responses API endpoint using Prober interface
func (ap *AdaptiveProbe) probeOpenAIResponsesEndpoint(ctx context.Context, provider *typ.Provider, modelID string) probe.EndpointStatus {
	startTime := time.Now()

	// Get OpenAI client from pool
	wrapper := ap.server.clientPool.GetOpenAIClient(context.Background(), provider, modelID)
	if wrapper == nil {
		return probe.EndpointStatus{
			Available:      false,
			SupportsStream: false,
			ErrorMessage:   "Failed to get OpenAI client",
			LastChecked:    time.Now(),
		}
	}

	// Create a context with timeout for the probe
	probeCtx, cancel := context.WithTimeout(ctx, DefaultProbeTimeout)
	defer cancel()

	// Explicitly probe the Responses endpoint.
	result, err := wrapper.ProbeResponsesEndpoint(probeCtx, modelID, client.ProbeEndpointOptions{Message: "Hi", Stream: true, Mode: client.ProbeModeStreaming})
	latency := int(time.Since(startTime).Milliseconds())

	if err != nil {
		return probe.EndpointStatus{
			Available:      false,
			SupportsStream: false,
			LatencyMs:      latency,
			ErrorMessage:   fmt.Sprintf("Responses probe failed: %v", err),
			LastChecked:    time.Now(),
		}
	}

	if result != nil && result.Content != "" {
		return probe.EndpointStatus{
			Available:      true,
			SupportsStream: true, // Responses API always supports streaming
			LatencyMs:      latency,
			ErrorMessage:   "",
			LastChecked:    time.Now(),
		}
	}

	return probe.EndpointStatus{
		Available:      false,
		SupportsStream: false,
		LatencyMs:      latency,
		ErrorMessage:   "Responses probe returned no content",
		LastChecked:    time.Now(),
	}
}

// determinePreferredEndpoint determines which endpoint to prefer based on availability and streaming support.
// Priority:
// 1. Chat API with streaming support (most stable and widely supported)
// 2. Responses API with streaming support (for specialized models like Codex)
// 3. Chat API without streaming (fallback for compatibility)
// 4. Responses API without streaming (last resort)
func (ap *AdaptiveProbe) determinePreferredEndpoint(chat, responses *probe.EndpointStatus) string {
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
func (ap *AdaptiveProbe) persistResult(result *probe.ProbeResult) {
	// Save chat endpoint capability
	err := ap.server.capabilityStore.SaveEndpointCapability(
		result.ProviderUUID,
		result.ModelID,
		db.EndpointTypeChat,
		result.ChatEndpoint.Available,
		result.ChatEndpoint.SupportsStream,
		result.ChatEndpoint.LatencyMs,
		result.ChatEndpoint.ErrorMessage,
	)
	if err != nil {
		logrus.Warnf("Failed to save chat capability: %v", err)
	}

	// Save responses endpoint capability
	err = ap.server.capabilityStore.SaveEndpointCapability(
		result.ProviderUUID,
		result.ModelID,
		db.EndpointTypeResponses,
		result.ResponsesEndpoint.Available,
		result.ResponsesEndpoint.SupportsStream,
		result.ResponsesEndpoint.LatencyMs,
		result.ResponsesEndpoint.ErrorMessage,
	)
	if err != nil {
		logrus.Warnf("Failed to save responses capability: %v", err)
	}
}

// cachedCapabilityToResult converts cached capability to probe result
func (ap *AdaptiveProbe) cachedCapabilityToResult(capability *probe.ModelEndpointCapability) *probe.ProbeResult {
	return &probe.ProbeResult{
		ProviderUUID: capability.ProviderUUID,
		ModelID:      capability.ModelID,
		ChatEndpoint: probe.EndpointStatus{
			Available:      capability.SupportsChat,
			SupportsStream: capability.ChatSupportsStream,
			LatencyMs:      capability.ChatLatencyMs,
			ErrorMessage:   capability.ChatError,
			LastChecked:    capability.LastVerified,
		},
		ResponsesEndpoint: probe.EndpointStatus{
			Available:      capability.SupportsResponses,
			SupportsStream: capability.ResponsesSupportsStream,
			LatencyMs:      capability.ResponsesLatencyMs,
			ErrorMessage:   capability.ResponsesError,
			LastChecked:    capability.LastVerified,
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
func (ap *AdaptiveProbe) GetModelCapability(providerUUID, modelID string) (*probe.ModelEndpointCapability, error) {
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
			chatStatus := probe.EndpointStatus{
				Available:      dbCapability.SupportsChat,
				SupportsStream: dbCapability.ChatSupportsStream,
				LatencyMs:      dbCapability.ChatLatencyMs,
				ErrorMessage:   dbCapability.ChatError,
				LastChecked:    dbCapability.LastVerified,
			}
			responsesStatus := probe.EndpointStatus{
				Available:      dbCapability.SupportsResponses,
				SupportsStream: dbCapability.ResponsesSupportsStream,
				LatencyMs:      dbCapability.ResponsesLatencyMs,
				ErrorMessage:   dbCapability.ResponsesError,
				LastChecked:    dbCapability.LastVerified,
			}

			// Recalculate PreferredEndpoint using current logic (NOT from database)
			// This ensures behavior changes take effect immediately
			preferredEndpoint := ap.determinePreferredEndpoint(&chatStatus, &responsesStatus)

			capability := &probe.ModelEndpointCapability{
				ProviderUUID:            dbCapability.ProviderUUID,
				ModelID:                 dbCapability.ModelID,
				SupportsChat:            dbCapability.SupportsChat,
				ChatSupportsStream:      dbCapability.ChatSupportsStream,
				ChatLatencyMs:           dbCapability.ChatLatencyMs,
				ChatError:               dbCapability.ChatError,
				SupportsResponses:       dbCapability.SupportsResponses,
				ResponsesSupportsStream: dbCapability.ResponsesSupportsStream,
				ResponsesLatencyMs:      dbCapability.ResponsesLatencyMs,
				ResponsesError:          dbCapability.ResponsesError,
				PreferredEndpoint:       preferredEndpoint, // Recalculated, not from DB
				LastVerified:            dbCapability.LastVerified,
			}

			// Cache the recalculated capability
			if ap.server.probeCache != nil {
				ap.server.probeCache.Set(providerUUID, modelID, capability)
			}

			// If data is stale, trigger async probe refresh
			if ap.server.probeCache != nil && time.Since(dbCapability.LastVerified) > ap.server.probeCache.TTL() {
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), DefaultProbeTimeout)
					defer cancel()
					ap.ProbeModelEndpoints(ctx, probe.ModelProbeRequest{
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
	// Codex providers only support Responses API — never probe or route to chat
	if provider.OAuthDetail != nil && provider.OAuthDetail.GetIssuer() == "codex" {
		return string(db.EndpointTypeResponses)
	}

	capability, err := ap.GetModelCapability(provider.UUID, modelID)
	if err != nil || capability.PreferredEndpoint == "" {
		// No cached data - trigger synchronous probe
		// This is the cold start path
		ctx, cancel := context.WithTimeout(context.Background(), DefaultProbeTimeout)
		defer cancel()
		res, err := ap.ProbeModelEndpoints(ctx, probe.ModelProbeRequest{
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
