package server

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
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
	s.updateServiceStats(rule, provider, model, inputTokens, outputTokens)

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
func (s *Server) updateServiceStats(rule *typ.Rule, provider *typ.Provider, model string, inputTokens, outputTokens int) {
	if rule == nil || provider == nil {
		return
	}

	// Find the matching service in the rule and update its stats
	for i := range rule.Services {
		service := rule.Services[i]
		if service.Active && service.Provider == provider.UUID && service.Model == model {
			service.RecordUsage(inputTokens, outputTokens)

			// Persist to stats store
			if statsStore := s.config.GetStatsStore(); statsStore != nil {
				_ = statsStore.UpdateFromService(service)
			}
			return
		}
	}
}
