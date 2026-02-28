package server

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/obs/otel"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// trackUsage records token usage using the UsageTracker.
// It will also record to OTel if the token tracker is available in the gin context.
//
// Deprecated: Use trackUsageFromContext instead. This method is kept for backward compatibility
// during the migration period and will be removed in Phase 2.5.
func (s *Server) trackUsage(c *gin.Context, rule *typ.Rule, provider *typ.Provider, model, requestModel string, inputTokens, outputTokens int, streamed bool, status, errorCode string) {
	// Set token tracker in context for RecordUsage to use
	if s.tokenTracker != nil {
		c.Set("token_tracker", s.tokenTracker)
	}

	tracker := s.NewUsageTracker()
	tracker.RecordUsage(c, rule, provider, model, requestModel, inputTokens, outputTokens, streamed, status, errorCode)
}

// trackUsageFromContext records token usage by extracting all metadata from the gin context.
// This is the new preferred method that eliminates explicit parameter passing.
//
// Parameters:
//   - c: Gin context containing all tracking metadata
//   - inputTokens: Number of input/prompt tokens consumed
//   - outputTokens: Number of output/completion tokens consumed
//   - err: Error if request failed, nil for success (context.Canceled maps to "canceled" status)
func (s *Server) trackUsageFromContext(c *gin.Context, inputTokens, outputTokens int, err error) {
	rule, provider, model, requestModel, scenario, streamed, startTime := GetTrackingContext(c)

	if rule == nil || provider == nil || model == "" {
		return
	}

	latencyMs := calculateLatencyFromStart(startTime)

	// Determine status and error code from error
	status, errorCode := "success", ""
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			status = "canceled"
			errorCode = "client_disconnected"
		} else {
			status = "error"
			errorCode = sanitizeErrorCode(err)
		}
	}

	// 1. Update service stats (inline, no UsageTracker allocation)
	s.updateServiceStats(rule, provider, model, inputTokens, outputTokens, latencyMs)

	// 2. Record to OTel (primary path for metrics)
	if s.tokenTracker != nil {
		s.tokenTracker.RecordUsage(c.Request.Context(), otel.UsageOptions{
			Provider:     provider.Name,
			ProviderUUID: provider.UUID,
			Model:        model,
			RequestModel: requestModel,
			RuleUUID:     rule.UUID,
			Scenario:     scenario,
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			Streamed:     streamed,
			Status:       status,
			ErrorCode:    errorCode,
			LatencyMs:    latencyMs,
		})
	}

	// 3. Record detailed usage (for analytics/dashboard)
	s.recordDetailedUsage(c, rule, provider, model, requestModel, scenario, inputTokens, outputTokens, streamed, status, errorCode, latencyMs)

	// 4. Report to health monitor for service health tracking
	s.reportHealthStatus(provider, model, err, errorCode)
}

// sanitizeErrorCode extracts a safe error code from an error.
func sanitizeErrorCode(err error) string {
	if err == nil {
		return ""
	}
	// Use error type name as code, avoid exposing sensitive info
	return err.Error()
}

// recordDetailedUsage writes a detailed usage record to the database.
// This maintains the detailed analytics tracking for the dashboard.
func (s *Server) recordDetailedUsage(c *gin.Context, rule *typ.Rule, provider *typ.Provider, model, requestModel, scenario string, inputTokens, outputTokens int, streamed bool, status, errorCode string, latencyMs int) {
	if s.config == nil {
		return
	}

	usageStore := s.config.GetUsageStore()
	if usageStore == nil {
		return
	}

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

	_ = usageStore.RecordUsage(record)
}

// updateServiceStats updates the service-level statistics for load balancing.
// This is inlined from the old UsageTracker.recordOnService to avoid unnecessary allocations.
func (s *Server) updateServiceStats(rule *typ.Rule, provider *typ.Provider, model string, inputTokens, outputTokens int, latencyMs int) {
	if rule == nil || provider == nil || s.config == nil {
		return
	}

	// Find the matching service in the rule and update its stats
	for i := range rule.Services {
		service := rule.Services[i]
		if service.Active && service.Provider == provider.UUID && service.Model == model {
			service.RecordUsage(inputTokens, outputTokens)

			// Record latency metrics for latency-based routing
			service.Stats.RecordLatency(int64(latencyMs), constant.DefaultLatencySampleWindow)

			// Record token speed metrics (tokens per second) for speed-based routing
			if latencyMs > 0 && outputTokens > 0 {
				speedTps := float64(outputTokens) / (float64(latencyMs) / 1000.0) // Convert ms to seconds
				service.Stats.RecordTokenSpeed(speedTps, constant.DefaultSpeedSampleWindow)
			}

			// Persist to stats store
			if statsStore := s.config.GetStatsStore(); statsStore != nil {
				_ = statsStore.UpdateFromService(service)
			}
			return
		}
	}
}

// TrackUsage implements the UsageTracker interface.
// It extracts the gin.Context from the provided context and calls trackUsageFromContext.
// The gin.Context should be stored in the context with the key "gin_context".
func (s *Server) TrackUsage(ctx context.Context, inputTokens, outputTokens int, err error) {
	// Try to get gin.Context from the context
	// This is set when creating HandleContext
	if c, ok := ctx.Value("gin_context").(*gin.Context); ok {
		s.trackUsageFromContext(c, inputTokens, outputTokens, err)
	}
}

// reportHealthStatus reports the health status of a service based on request outcome.
// It uses the health monitor to track service health for load balancing decisions.
func (s *Server) reportHealthStatus(provider *typ.Provider, model string, err error, errorCode string) {
	if s.healthMonitor == nil || provider == nil || model == "" {
		return
	}

	serviceID := fmt.Sprintf("%s:%s", provider.UUID, model)

	if err == nil {
		// Success - report to health monitor
		s.healthMonitor.ReportSuccess(serviceID)
		return
	}

	// Error - classify and report appropriately
	errStr := err.Error()

	// Check for rate limit (429)
	if strings.Contains(errStr, "429") || strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "RateLimit") {
		s.healthMonitor.ReportRateLimit(serviceID)
		return
	}

	// Check for auth errors (401/403)
	if strings.Contains(errStr, "401") || strings.Contains(errStr, "403") ||
		strings.Contains(errStr, "unauthorized") || strings.Contains(errStr, "forbidden") {
		// Try to determine if it's 401 or 403
		if strings.Contains(errStr, "401") {
			s.healthMonitor.ReportAuthError(serviceID, 401)
		} else {
			s.healthMonitor.ReportAuthError(serviceID, 403)
		}
		return
	}

	// Check for retryable errors (timeout, connection refused)
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") || strings.Contains(errStr, "i/o timeout") {
		s.healthMonitor.ReportError(serviceID, err)
		return
	}

	// For other errors, report as general error
	s.healthMonitor.ReportError(serviceID, err)
}
