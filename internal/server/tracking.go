package server

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"tingly-box/internal/db"
	"tingly-box/internal/typ"
)

// UsageTracker provides usage tracking methods for handlers.
// It encapsulates the logic for recording token usage to both service stats
// and detailed usage records.
type UsageTracker struct {
	statsStore *db.StatsStore
	usageStore *db.UsageStore
}

// NewUsageTracker creates a new UsageTracker
func (s *Server) NewUsageTracker() *UsageTracker {
	return &UsageTracker{
		statsStore: s.config.GetStatsStore(),
		usageStore: s.config.GetUsageStore(),
	}
}

// RecordUsage records token usage from a handler.
// It updates both the service-level stats and the detailed usage records.
//
// Parameters:
//   - c: Gin context for accessing request metadata
//   - rule: The load balancer rule that was used
//   - provider: The provider that handled the request
//   - model: The actual model name used (not the requested model)
//   - requestModel: The original model name requested by the user
//   - inputTokens: Number of input/prompt tokens consumed
//   - outputTokens: Number of output/completion tokens consumed
//   - streamed: Whether this was a streaming request
//   - status: Request status - "success", "error", or "partial"
//   - errorCode: Error code if status is not "success"
func (t *UsageTracker) RecordUsage(
	c *gin.Context,
	rule *typ.Rule,
	provider *typ.Provider,
	model, requestModel string,
	inputTokens, outputTokens int,
	streamed bool,
	status, errorCode string,
) {
	if t == nil || rule == nil || provider == nil {
		return
	}

	// 1. Record usage on the rule's service stats (for load balancing)
	t.recordOnService(rule, provider, model, inputTokens, outputTokens)

	// 2. Record detailed usage (for analytics/dashboard)
	if t.usageStore != nil {
		t.recordDetailed(c, rule, provider, model, requestModel, inputTokens, outputTokens, streamed, status, errorCode)
	}
}

// recordOnService updates the service-level statistics for load balancing
func (t *UsageTracker) recordOnService(rule *typ.Rule, provider *typ.Provider, model string, inputTokens, outputTokens int) {
	// Find the matching service in the rule and update its stats
	for i := range rule.Services {
		service := &rule.Services[i]
		if service.Active && service.Provider == provider.UUID && service.Model == model {
			service.RecordUsage(inputTokens, outputTokens)

			// Persist to stats store
			if t.statsStore != nil {
				_ = t.statsStore.UpdateFromService(service)
			}
			return
		}
	}
}

// recordDetailed writes a detailed usage record to the database
func (t *UsageTracker) recordDetailed(
	c *gin.Context,
	rule *typ.Rule,
	provider *typ.Provider,
	model, requestModel string,
	inputTokens, outputTokens int,
	streamed bool,
	status, errorCode string,
) {
	scenario := extractScenarioFromPath(c.Request.URL.Path)
	latencyMs := calculateLatency(c)

	record := &db.UsageRecord{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		Model:        model,
		Scenario:     scenario,
		RequestModel: requestModel,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  inputTokens + outputTokens,
		Status:       status,
		ErrorCode:    errorCode,
		LatencyMs:    latencyMs,
		Streamed:     streamed,
	}

	if rule != nil {
		record.RuleUUID = rule.UUID
	}

	_ = t.usageStore.RecordUsage(record)
}

// extractScenarioFromPath extracts the scenario from the request path
func extractScenarioFromPath(path string) string {
	if strings.Contains(path, "/openai/") {
		return "openai"
	}
	if strings.Contains(path, "/anthropic/") {
		return "anthropic"
	}
	if strings.Contains(path, "/claude_code/") || strings.Contains(path, "/claude-code/") {
		return "claude_code"
	}
	if strings.Contains(path, "/tingly/") {
		// Extract scenario from tingly path
		parts := strings.Split(path, "/")
		for i, part := range parts {
			if part == "tingly" && i+1 < len(parts) {
				return parts[i+1]
			}
		}
	}
	return "unknown"
}

// calculateLatency calculates the request processing time in milliseconds
func calculateLatency(c *gin.Context) int {
	// Try to get start time from context
	if start, exists := c.Get("start_time"); exists {
		if startFloat, ok := start.(float64); ok {
			elapsed := time.Now().UnixNano() - int64(startFloat)
			return int(elapsed / 1000000) // Convert nanoseconds to milliseconds
		}
	}
	return 0
}
