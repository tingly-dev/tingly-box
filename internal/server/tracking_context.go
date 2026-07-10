package server

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Gin context key aliases — canonical values live in constant so that
// routing/ and middleware/ sub-packages can reference them without an
// import cycle.
const (
	ContextKeyRule           = constant.CtxKeyRule
	ContextKeyProvider       = constant.CtxKeyProvider
	ContextKeyModel          = constant.CtxKeyModel
	ContextKeyRequestModel   = constant.CtxKeyRequestModel
	ContextKeyScenario       = constant.CtxKeyScenario
	ContextKeyStreamed       = constant.CtxKeyStreamed
	ContextKeyStartTime      = constant.CtxKeyStartTime
	ContextKeyFirstTokenTime = constant.CtxKeyFirstTokenTime
	ContextKeyCacheHit       = constant.CtxKeyCacheHit
	ContextKeySessionID      = constant.CtxKeySessionID
	ContextKeyAffinityKey    = constant.CtxKeyAffinityKey
	ContextKeyLBServiceID    = constant.CtxKeyLBServiceID
	ContextKeyLBTactic       = constant.CtxKeyLBTactic
)

// SetTrackingContext sets all tracking metadata in the gin context.
// This should be called once at the beginning of request processing
// to avoid explicit parameter passing throughout the handler chain.
//
// Parameters:
//   - c: Gin context
//   - rule: The load balancer rule that was selected
//   - provider: The provider that was selected
//   - actualModel: The actual model name used (may differ from requested)
//   - requestModel: The original model name requested by the user
//   - streamed: Whether this is a streaming request
func SetTrackingContext(c *gin.Context, rule *typ.Rule, provider *typ.Provider, actualModel, requestModel string, streamed bool) {
	c.Set(ContextKeyRule, rule)
	c.Set(ContextKeyProvider, provider)
	c.Set(ContextKeyModel, actualModel)
	c.Set(ContextKeyRequestModel, requestModel)
	c.Set(ContextKeyStreamed, streamed)
	c.Set(ContextKeyStartTime, time.Now())

	// Extract scenario from path if not already set
	if _, exists := c.Get(ContextKeyScenario); !exists {
		scenario := "unknown"
		if c.Request != nil && c.Request.URL != nil {
			scenario = ExtractScenarioFromPath(c.Request.URL.Path)
		}
		c.Set(ContextKeyScenario, scenario)
	}
}

// GetTrackingContext retrieves tracking metadata from the gin context.
// Returns zero values if the context keys are not set.
func GetTrackingContext(c *gin.Context) (rule *typ.Rule, provider *typ.Provider, actualModel, requestModel, scenario string, streamed bool, startTime time.Time) {
	if r, exists := c.Get(ContextKeyRule); exists {
		rule = r.(*typ.Rule)
	}
	if p, exists := c.Get(ContextKeyProvider); exists {
		provider = p.(*typ.Provider)
	}
	if m, exists := c.Get(ContextKeyModel); exists {
		actualModel = m.(string)
	}
	if rm, exists := c.Get(ContextKeyRequestModel); exists {
		requestModel = rm.(string)
	}
	if s, exists := c.Get(ContextKeyScenario); exists {
		scenario = s.(string)
	}
	if st, exists := c.Get(ContextKeyStreamed); exists {
		streamed = st.(bool)
	}
	if t, exists := c.Get(ContextKeyStartTime); exists {
		startTime = t.(time.Time)
	}
	return
}

func GetTrackingContextScenario(c *gin.Context) (scenario string) {
	if s, exists := c.Get(ContextKeyScenario); exists {
		scenario = s.(string)
	}
	return
}

// CalculateLatencyFromStart calculates the elapsed time in milliseconds since the start time.
func CalculateLatencyFromStart(startTime time.Time) int {
	if startTime.IsZero() {
		return 0
	}
	elapsed := time.Since(startTime)
	return int(elapsed.Milliseconds())
}

// GetFirstTokenTime retrieves the first token time from the context.
// Returns the timestamp and true if it exists, zero time and false otherwise.
func GetFirstTokenTime(c *gin.Context) (time.Time, bool) {
	if val, exists := c.Get(ContextKeyFirstTokenTime); exists {
		if t, ok := val.(time.Time); ok {
			return t, true
		}
	}
	return time.Time{}, false
}

// CalculateTTFT returns Time To First Token in milliseconds, or 0 when no
// first-token time was recorded (e.g. non-streaming requests). It must not
// fall back to total latency, which would make TTFT indistinguishable from it.
func CalculateTTFT(c *gin.Context) int64 {
	startTime := time.Time{}
	if t, exists := c.Get(ContextKeyStartTime); exists {
		if st, ok := t.(time.Time); ok {
			startTime = st
		}
	}

	if startTime.IsZero() {
		return 0
	}

	if firstTokenTime, hasTTFT := GetFirstTokenTime(c); hasTTFT {
		return firstTokenTime.Sub(startTime).Milliseconds()
	}

	return 0
}

// SetCacheHit records whether this request was a cache hit.
func SetCacheHit(c *gin.Context, isHit bool) {
	c.Set(ContextKeyCacheHit, isHit)
}

// UpdateTrackingForFailover updates provider and model tracking during failover retry.
// This ensures that logging/middleware shows the final successful service after failover,
// not the initially-selected failed service.
//
// Parameters:
//   - c: Gin context
//   - provider: The new provider being tried in this failover attempt
//   - model: The new model being tried in this failover attempt
func UpdateTrackingForFailover(c *gin.Context, provider *typ.Provider, model string) {
	c.Set(ContextKeyProvider, provider)
	c.Set(ContextKeyModel, model)
	c.Set(ContextKeyLBServiceID, provider.UUID+"/"+model)
}

// GetCacheHit retrieves the cache hit status from the context.
// Returns the cache hit status and true if it exists, false and false otherwise.
func GetCacheHit(c *gin.Context) (bool, bool) {
	if val, exists := c.Get(ContextKeyCacheHit); exists {
		if hit, ok := val.(bool); ok {
			return hit, true
		}
	}
	return false, false
}

// CalculateTPS calculates Tokens Per Second (generation speed) for streaming requests.
// For non-streaming requests or when TTFT is not available, returns 0.
//
// TPS is calculated as: outputTokens / (currentTime - firstTokenTime)
// This measures the actual token generation speed after the first token was received.
//
// Parameters:
//   - c: Gin context containing timing information
//   - outputTokens: Number of output tokens generated
//   - streamed: Whether this was a streaming request
//
// Returns:
//   - TPS value (tokens per second), or 0 if not applicable
func CalculateTPS(c *gin.Context, outputTokens int, streamed bool) float64 {
	// TPS only makes sense for streaming requests
	if !streamed {
		return 0
	}

	// Need output tokens to calculate
	if outputTokens <= 0 {
		return 0
	}

	// Get first token time - required for accurate TPS calculation
	firstTokenTime, hasFirstToken := GetFirstTokenTime(c)
	if !hasFirstToken {
		return 0 // Cannot calculate TPS without knowing when first token arrived
	}

	// Calculate duration from first token to now (in seconds)
	duration := time.Since(firstTokenTime).Seconds()
	if duration <= 0 {
		return 0 // Invalid duration
	}

	// TPS = tokens / seconds
	return float64(outputTokens) / duration
}

// DetectCacheHit determines if a request was served from cache based on TokenUsage.
// Returns true if cache was hit, false otherwise.
//
// Detection logic:
//   - OpenAI: usage.CacheInputTokens > 0 (cache_read_input_tokens field)
//   - Anthropic: usage.CacheInputTokens > 0 (cache_read_input_tokens field)
//   - Other providers: Returns false (conservative - assumes cache miss)
//
// Parameters:
//   - usage: Token usage information from the response
//
// Returns:
//   - true if cache hit detected, false otherwise
func DetectCacheHit(usage *protocol.TokenUsage) bool {
	if usage == nil {
		return false
	}

	// OpenAI and Anthropic both expose cache tokens via CacheInputTokens field
	// If cache tokens > 0, it means cache was hit
	return usage.CacheInputTokens > 0
}
