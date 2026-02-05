package server

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Gin context keys for tracking metadata.
// These keys are used to store tracking information in the gin context
// to avoid explicit parameter passing throughout the handler chain.
const (
	ContextKeyRule         = "tracking_rule"          // *typ.Rule
	ContextKeyProvider     = "tracking_provider"      // *typ.Provider
	ContextKeyModel        = "tracking_model"         // string (actual model used)
	ContextKeyRequestModel = "tracking_request_model" // string (model requested by user)
	ContextKeyScenario     = "tracking_scenario"      // string (extracted from request path)
	ContextKeyStreamed     = "tracking_streamed"      // bool
	ContextKeyStartTime    = "tracking_start_time"    // time.Time
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
		scenario := extractScenarioFromPath(c.Request.URL.Path)
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

// calculateLatencyFromStart calculates the elapsed time in milliseconds since the start time.
func calculateLatencyFromStart(startTime time.Time) int {
	if startTime.IsZero() {
		return 0
	}
	elapsed := time.Since(startTime)
	return int(elapsed.Milliseconds())
}
