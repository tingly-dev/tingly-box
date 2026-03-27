package server

import (
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestSetTrackingContext_SetsAllValues(t *testing.T) {
	c := &gin.Context{}

	rule := &typ.Rule{UUID: "test-rule"}
	provider := &typ.Provider{UUID: "test-provider", Name: "test"}
	actualModel := "gpt-4"
	requestModel := "gpt-4"
	streamed := true

	SetTrackingContext(c, rule, provider, actualModel, requestModel, streamed)

	// Verify rule
	ruleVal, exists := c.Get(ContextKeyRule)
	assert.True(t, exists)
	assert.Equal(t, rule, ruleVal.(*typ.Rule))

	// Verify provider
	providerVal, exists := c.Get(ContextKeyProvider)
	assert.True(t, exists)
	assert.Equal(t, provider, providerVal.(*typ.Provider))

	// Verify model
	modelVal, exists := c.Get(ContextKeyModel)
	assert.True(t, exists)
	assert.Equal(t, actualModel, modelVal.(string))

	// Verify request model
	requestModelVal, exists := c.Get(ContextKeyRequestModel)
	assert.True(t, exists)
	assert.Equal(t, requestModel, requestModelVal.(string))

	// Verify streamed
	streamedVal, exists := c.Get(ContextKeyStreamed)
	assert.True(t, exists)
	assert.True(t, streamedVal.(bool))

	// Verify start time
	startTimeVal, exists := c.Get(ContextKeyStartTime)
	assert.True(t, exists)
	assert.False(t, startTimeVal.(time.Time).IsZero())
}

func TestGetTrackingContext_WithValues(t *testing.T) {
	c := &gin.Context{}

	rule := &typ.Rule{UUID: "test-rule"}
	provider := &typ.Provider{UUID: "test-provider", Name: "test"}
	actualModel := "gpt-4"
	requestModel := "gpt-4"
	streamed := true

	SetTrackingContext(c, rule, provider, actualModel, requestModel, streamed)

	rule, provider, model, requestModel, scenario, streamed, startTime := GetTrackingContext(c)

	assert.NotNil(t, rule)
	assert.Equal(t, "test-rule", rule.UUID)
	assert.NotNil(t, provider)
	assert.Equal(t, "test-provider", provider.UUID)
	assert.Equal(t, "gpt-4", model)
	assert.Equal(t, "gpt-4", requestModel)
	assert.Equal(t, "unknown", scenario) // No URL path set, so scenario is "unknown"
	assert.True(t, streamed)
	assert.False(t, startTime.IsZero())
}

func TestGetTrackingContext_EmptyContext(t *testing.T) {
	c := &gin.Context{}

	rule, provider, model, requestModel, scenario, streamed, startTime := GetTrackingContext(c)

	assert.Nil(t, rule)
	assert.Nil(t, provider)
	assert.Empty(t, model)
	assert.Empty(t, requestModel)
	assert.Empty(t, scenario)
	assert.False(t, streamed)
	assert.True(t, startTime.IsZero())
}

func TestCalculateLatencyFromStart(t *testing.T) {
	t.Run("100ms latency", func(t *testing.T) {
		start := time.Now().Add(-100 * time.Millisecond)
		latency := calculateLatencyFromStart(start)

		// Allow some tolerance for execution time
		assert.GreaterOrEqual(t, latency, 95)
		assert.Less(t, latency, 200)
	})

	t.Run("zero time", func(t *testing.T) {
		latency := calculateLatencyFromStart(time.Time{})
		assert.Equal(t, 0, latency)
	})
}

func TestExtractScenarioFromPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"OpenAI path", "/v1/openai/chat/completions", "openai"},
		{"Codex path", "/v1/codex/responses", "codex"},
		{"Anthropic path", "/v1/anthropic/messages", "anthropic"},
		{"Claude Code path", "/v1/claude_code/messages", "claude_code"},
		{"Claude Code with dash path", "/v1/claude-code/messages", "claude_code"},
		{"Tingly path", "/v1/tingly/custom/messages", "custom"},
		{"Unknown path", "/v1/unknown/endpoint", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractScenarioFromPath(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCalculateTPS_Streaming tests TPS calculation for streaming requests
func TestCalculateTPS_Streaming(t *testing.T) {
	c := &gin.Context{}

	// Set up context: streaming request with first token time 500ms ago
	c.Set(ContextKeyStreamed, true)
	c.Set(ContextKeyFirstTokenTime, time.Now().Add(-500*time.Millisecond))

	// Calculate TPS with 100 output tokens
	tps := CalculateTPS(c, 100, true)

	// Expected: 100 tokens / 0.5 seconds = 200 TPS
	// Allow tolerance for execution time (180-220)
	assert.GreaterOrEqual(t, tps, 180.0)
	assert.LessOrEqual(t, tps, 220.0)
}

// TestCalculateTPS_NonStreaming tests that non-streaming requests return 0 TPS
func TestCalculateTPS_NonStreaming(t *testing.T) {
	c := &gin.Context{}

	// Non-streaming request
	tps := CalculateTPS(c, 100, false)

	// Should return 0 for non-streaming
	assert.Equal(t, 0.0, tps)
}

// TestCalculateTPS_NoFirstToken tests TPS calculation without first token time
func TestCalculateTPS_NoFirstToken(t *testing.T) {
	c := &gin.Context{}

	// Streaming but no first token time set
	tps := CalculateTPS(c, 100, true)

	// Should return 0 when first token time is not available
	assert.Equal(t, 0.0, tps)
}

// TestCalculateTPS_ZeroTokens tests TPS calculation with zero output tokens
func TestCalculateTPS_ZeroTokens(t *testing.T) {
	c := &gin.Context{}

	c.Set(ContextKeyFirstTokenTime, time.Now().Add(-500*time.Millisecond))

	// Zero output tokens
	tps := CalculateTPS(c, 0, true)

	// Should return 0 for zero tokens
	assert.Equal(t, 0.0, tps)
}

// TestCalculateTPS_NegativeTokens tests TPS calculation with negative tokens (edge case)
func TestCalculateTPS_NegativeTokens(t *testing.T) {
	c := &gin.Context{}

	c.Set(ContextKeyFirstTokenTime, time.Now().Add(-500*time.Millisecond))

	// Negative tokens (shouldn't happen but test for safety)
	tps := CalculateTPS(c, -10, true)

	// Should return 0 for invalid token count
	assert.Equal(t, 0.0, tps)
}

// TestDetectCacheHit tests cache hit detection
func TestDetectCacheHit(t *testing.T) {
	t.Run("cache hit", func(t *testing.T) {
		usage := &protocol.TokenUsage{
			InputTokens:      100,
			OutputTokens:     50,
			CacheInputTokens: 80, // Cache was used
		}

		cacheHit := detectCacheHit(usage)
		assert.True(t, cacheHit)
	})

	t.Run("cache miss", func(t *testing.T) {
		usage := &protocol.TokenUsage{
			InputTokens:      100,
			OutputTokens:     50,
			CacheInputTokens: 0, // No cache
		}

		cacheHit := detectCacheHit(usage)
		assert.False(t, cacheHit)
	})

	t.Run("nil usage", func(t *testing.T) {
		cacheHit := detectCacheHit(nil)
		assert.False(t, cacheHit)
	})
}

// TestCalculateTTFT tests TTFT calculation
func TestCalculateTTFT(t *testing.T) {
	t.Run("streaming with first token time", func(t *testing.T) {
		c := &gin.Context{}

		startTime := time.Now().Add(-500 * time.Millisecond)
		firstTokenTime := time.Now().Add(-200 * time.Millisecond)

		c.Set(ContextKeyStartTime, startTime)
		c.Set(ContextKeyFirstTokenTime, firstTokenTime)

		ttft := CalculateTTFT(c)

		// Expected: ~300ms (500ms - 200ms)
		// Allow tolerance (250-350ms)
		assert.GreaterOrEqual(t, ttft, int64(250))
		assert.LessOrEqual(t, ttft, int64(350))
	})

	t.Run("non-streaming fallback to total latency", func(t *testing.T) {
		c := &gin.Context{}

		startTime := time.Now().Add(-500 * time.Millisecond)
		c.Set(ContextKeyStartTime, startTime)
		// No first token time set

		ttft := CalculateTTFT(c)

		// Should fallback to total latency (~500ms)
		// Allow tolerance (450-550ms)
		assert.GreaterOrEqual(t, ttft, int64(450))
		assert.LessOrEqual(t, ttft, int64(550))
	})

	t.Run("no start time", func(t *testing.T) {
		c := &gin.Context{}

		ttft := CalculateTTFT(c)

		// Should return 0 when no start time
		assert.Equal(t, int64(0), ttft)
	})
}

// TestSetGetFirstTokenTime tests first token time tracking
func TestSetGetFirstTokenTime(t *testing.T) {
	c := &gin.Context{}

	// Initially should not exist
	_, exists := GetFirstTokenTime(c)
	assert.False(t, exists)

	// Set first token time
	now := time.Now()
	SetFirstTokenTime(c)

	// Should exist now
	firstTokenTime, exists := GetFirstTokenTime(c)
	assert.True(t, exists)
	assert.False(t, firstTokenTime.IsZero())
	assert.WithinDuration(t, now, firstTokenTime, 100*time.Millisecond)
}

// TestSetGetCacheHit tests cache hit tracking
func TestSetGetCacheHit(t *testing.T) {
	c := &gin.Context{}

	// Initially should not exist
	_, exists := GetCacheHit(c)
	assert.False(t, exists)

	// Set cache hit = true
	SetCacheHit(c, true)
	cacheHit, exists := GetCacheHit(c)
	assert.True(t, exists)
	assert.True(t, cacheHit)

	// Set cache hit = false
	SetCacheHit(c, false)
	cacheHit, exists = GetCacheHit(c)
	assert.True(t, exists)
	assert.False(t, cacheHit)
}
