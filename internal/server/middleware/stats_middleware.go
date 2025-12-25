package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"tingly-box/internal/config"

	"github.com/gin-gonic/gin"
)

// StatsMiddleware tracks usage statistics by updating service-embedded stats
type StatsMiddleware struct {
	config     *config.Config     // Reference to config to access config and rules
	statsStore *config.StatsStore // Dedicated stats store for persistence
}

// NewStatsMiddleware creates a new statistics middleware
func NewStatsMiddleware(cfg *config.Config) *StatsMiddleware {
	return &StatsMiddleware{
		config:     cfg,
		statsStore: cfg.GetStatsStore(),
	}
}

// Stop stops the cleanup routine (no-op in new architecture)
func (sm *StatsMiddleware) Stop() {
	// No-op: services handle their own cleanup
}

// Middleware returns the Gin middleware function
func (sm *StatsMiddleware) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only track responses for chat completion endpoints
		if !sm.shouldTrackEndpoint(c.Request.URL.Path, c.Request.Method) {
			c.Next()
			return
		}

		// Capture response body
		responseWriter := &responseBodyWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
		}
		c.Writer = responseWriter

		// Process the request
		c.Next()

		// Extract usage information from response
		sm.extractAndRecordUsage(c, responseWriter.body.String())
	}
}

// shouldTrackEndpoint checks if we should track statistics for this endpoint
func (sm *StatsMiddleware) shouldTrackEndpoint(path, method string) bool {
	// Track POST requests to chat completion endpoints
	if method != "POST" {
		return false
	}

	return strings.HasSuffix(path, "/chat/completions") ||
		strings.HasSuffix(path, "/messages")
}

// extractAndRecordUsage extracts token usage from response and records it
func (sm *StatsMiddleware) extractAndRecordUsage(c *gin.Context, responseBody string) {
	// Get the provider and model information from context
	provider, model := sm.getProviderModelFromContext(c)
	if provider == "" || model == "" {
		return
	}

	// Get the rule information from context (set by handlers)
	if rule, exists := c.Get("rule"); exists {
		if rulePtr, ok := rule.(*config.Rule); ok {
			// Record usage directly on the rule's services (same rule as handler used)
			inputTokens, outputTokens := sm.extractTokenUsage(responseBody, c.Request.URL.Path)
			sm.RecordUsageOnRule(rulePtr, provider, model, inputTokens, outputTokens)
			return
		}
	}

	// Fallback: search by provider/model (old behavior)
	serviceID := fmt.Sprintf("%s:%s", provider, model)
	inputTokens, outputTokens := sm.extractTokenUsage(responseBody, c.Request.URL.Path)
	sm.RecordUsage(serviceID, inputTokens, outputTokens)
}

// getProviderModelFromContext extracts provider and model from Gin context
func (sm *StatsMiddleware) getProviderModelFromContext(c *gin.Context) (provider, model string) {
	// Try to get from context set by handlers
	if p, exists := c.Get("provider"); exists {
		if provider, ok := p.(string); ok {
			// Get model from request body if available
			if modelFromContext := sm.getModelFromContext(c); modelFromContext != "" {
				return provider, modelFromContext
			}
		}
	}

	// Fallback: try to extract from request/response
	return sm.extractFromRequestResponse(c)
}

// getModelFromContext extracts model name from Gin context
func (sm *StatsMiddleware) getModelFromContext(c *gin.Context) string {
	if m, exists := c.Get("model"); exists {
		if model, ok := m.(string); ok {
			return model
		}
	}
	return ""
}

// extractFromRequestResponse attempts to extract provider and model from request/response
func (sm *StatsMiddleware) extractFromRequestResponse(c *gin.Context) (provider, model string) {
	// Read the request body if available
	if c.Request.Body != nil {
		body, err := io.ReadAll(c.Request.Body)
		if err == nil {
			// Restore the body for subsequent handlers
			c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

			// Parse JSON to extract model
			var request map[string]interface{}
			if json.Unmarshal(body, &request) == nil {
				if modelValue, ok := request["model"].(string); ok {
					model = modelValue
				}
			}
		}
	}

	return provider, model
}

// extractTokenUsage extracts input and output token consumption from response body
func (sm *StatsMiddleware) extractTokenUsage(responseBody, endpoint string) (int, int) {
	if responseBody == "" {
		return 0, 0
	}

	var response map[string]interface{}
	if err := json.Unmarshal([]byte(responseBody), &response); err != nil {
		return 0, 0
	}

	// Try to extract usage information from different response formats
	if usage, ok := response["usage"].(map[string]interface{}); ok {
		// OpenAI format - try to get separate input/output if available
		var inputTokens, outputTokens int

		if inputTok, ok := usage["prompt_tokens"].(float64); ok {
			inputTokens = int(inputTok)
		}
		if outputTok, ok := usage["completion_tokens"].(float64); ok {
			outputTokens = int(outputTok)
		}

		// If we got both, return them
		if inputTokens > 0 || outputTokens > 0 {
			return inputTokens, outputTokens
		}

		// Anthropic format
		if inputTok, ok := usage["input_tokens"].(float64); ok {
			inputTokens = int(inputTok)
		}
		if outputTok, ok := usage["output_tokens"].(float64); ok {
			outputTokens = int(outputTok)
		}

		// If we got both, return them
		if inputTokens > 0 || outputTokens > 0 {
			return inputTokens, outputTokens
		}

		// Fallback to total tokens
		if totalTokens, ok := usage["total_tokens"].(float64); ok {
			// Split evenly as approximation
			total := int(totalTokens)
			return total / 2, total - total/2
		}
	}

	// Alternative: estimate tokens (very rough approximation)
	// This is a fallback when usage information is not available
	totalEstimated := len(responseBody) / 4 // Rough estimate: 1 token ≈ 4 characters
	return totalEstimated / 2, totalEstimated - totalEstimated/2
}

// RecordUsage records usage for a service by finding it in the rules and updating its embedded stats
func (sm *StatsMiddleware) RecordUsage(serviceID string, inputTokens, outputTokens int) {
	if sm.config == nil {
		return
	}

	// Parse serviceID to get provider and model
	parts := strings.SplitN(serviceID, ":", 2)
	if len(parts) != 2 {
		return
	}
	provider, model := parts[0], parts[1]

	// Find the rule that contains this service and update it directly in the config
	rules := sm.config.GetRequestConfigs()
	for ruleIdx := range rules {
		rule := &rules[ruleIdx] // Get pointer to actual rule in config
		if !rule.Active {
			continue
		}

		// Look through the services to find the matching one
		for i := range rule.Services {
			service := &rule.Services[i]
			if service.Active && service.Provider == provider && service.Model == model {
				// Found the service, record usage in its embedded stats
				service.RecordUsage(inputTokens, outputTokens)

				// Persist usage stats separately from config
				sm.persistServiceStats(service)
				return
			}
		}
	}
}

// RecordUsageOnRule records usage directly on a specific rule's services
func (sm *StatsMiddleware) RecordUsageOnRule(rule *config.Rule, provider, model string, inputTokens, outputTokens int) {
	// Look through the services in the specific rule to find the matching one
	for i := range rule.Services {
		service := &rule.Services[i]
		if service.Active && service.Provider == provider && service.Model == model {
			// Found the service, record usage in its embedded stats
			service.RecordUsage(inputTokens, outputTokens)

			// Persist usage stats separately from config
			sm.persistServiceStats(service)
			return
		}
	}
}

// persistServiceStats writes the updated stats into the dedicated stats store.
func (sm *StatsMiddleware) persistServiceStats(service *config.Service) {
	if sm.statsStore == nil {
		return
	}
	_ = sm.statsStore.UpdateFromService(service)
}
