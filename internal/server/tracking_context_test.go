package server

import (
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

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
