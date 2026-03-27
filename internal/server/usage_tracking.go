package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/otel/tracker"
)

// MetricsData encapsulates all metrics collected for a request.
// This structure is used to pass comprehensive metrics to updateServiceStats
// without requiring frequent function signature changes.
type MetricsData struct {
	InputTokens  int     // Number of input/prompt tokens
	OutputTokens int     // Number of output/completion tokens
	LatencyMs    int64   // Total request latency in milliseconds
	TTFTMs       int64   // Time To First Token in milliseconds (0 if not available/applicable)
	CacheHit     bool    // Whether this request hit the cache
	TPS          float64 // Tokens Per Second - generation speed (0 for non-streaming requests)
}

var enterpriseRateLimitHook struct {
	mu       sync.RWMutex
	reporter func(context.Context, string, string, string, string) error
}

// SetEnterpriseRateLimitReporter sets callback for enterprise 429 events.
func SetEnterpriseRateLimitReporter(reporter func(context.Context, string, string, string, string) error) {
	enterpriseRateLimitHook.mu.Lock()
	defer enterpriseRateLimitHook.mu.Unlock()
	enterpriseRateLimitHook.reporter = reporter
}

func reportEnterpriseRateLimitEvent(ctx context.Context, keyPrefix, providerID, scenario, userID string) error {
	enterpriseRateLimitHook.mu.RLock()
	reporter := enterpriseRateLimitHook.reporter
	enterpriseRateLimitHook.mu.RUnlock()
	if reporter == nil {
		return nil
	}
	return reporter(ctx, keyPrefix, providerID, scenario, userID)
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

	// Collect all metrics from context
	ttftMs := CalculateTTFT(c)
	cacheHit, _ := GetCacheHit(c) // Default false if not set
	tps := CalculateTPS(c, outputTokens, streamed)

	// Build comprehensive metrics data
	metrics := MetricsData{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		LatencyMs:    int64(latencyMs),
		TTFTMs:       ttftMs,
		CacheHit:     cacheHit,
		TPS:          tps,
	}

	// 1. Update service stats with comprehensive metrics
	s.updateServiceStats(rule, provider, model, metrics)

	// 2. Record to OTel (primary path for metrics)
	if s.tokenTracker != nil {
		userTier := ""
		if strings.TrimSpace(c.GetString("enterprise_user_id")) != "" {
			userTier = "enterprise"
		}
		s.tokenTracker.RecordUsage(c.Request.Context(), tracker.UsageOptions{
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
			UserTier:     userTier,
		})
	}

	// 3. Record detailed usage (for analytics/dashboard)
	s.recordDetailedUsage(c, rule, provider, model, requestModel, scenario, inputTokens, outputTokens, streamed, status, errorCode, latencyMs)

	// 4. Report to health monitor for service health tracking
	s.reportHealthStatus(provider, model, err, errorCode)

	// 5. Enterprise key-level 429 alerting hook (best-effort).
	if err != nil && isRateLimitError(err) && strings.TrimSpace(c.GetString("enterprise_user_id")) != "" {
		_ = reportEnterpriseRateLimitEvent(
			c.Request.Context(),
			c.GetString("enterprise_key_prefix"),
			provider.UUID,
			scenario,
			c.GetString("enterprise_user_id"),
		)
	}
}

// trackUsageWithTokenUsage records comprehensive token usage using the TokenUsage structure.
// This method supports cache tokens and system tokens for complete usage tracking.
//
// Parameters:
//   - c: Gin context containing all tracking metadata
//   - usage: Comprehensive token usage including cache and system tokens
//   - err: Error if request failed, nil for success
func (s *Server) trackUsageWithTokenUsage(c *gin.Context, usage *protocol.TokenUsage, err error) {
	rule, provider, model, requestModel, scenario, streamed, startTime := GetTrackingContext(c)

	if rule == nil || provider == nil || model == "" || usage == nil {
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

	logrus.WithFields(logrus.Fields{
		"provider":      provider.Name,
		"model":         model,
		"scenario":      scenario,
		"input_tokens":  usage.InputTokens,
		"output_tokens": usage.OutputTokens,
		"cache_tokens":  usage.CacheInputTokens,
		"system_tokens": usage.SystemTokens,
		"total_tokens":  usage.TotalTokens(),
		"status":        status,
		"streamed":      streamed,
		"latency_ms":    latencyMs,
	}).Trace("trackUsageWithTokenUsage: recording token usage")

	// Detect cache hit from usage data and set in context
	cacheHit := detectCacheHit(usage)
	SetCacheHit(c, cacheHit)

	// Collect all metrics from context and usage
	ttftMs := CalculateTTFT(c)
	tps := CalculateTPS(c, usage.OutputTokens, streamed)

	// Build comprehensive metrics data
	metrics := MetricsData{
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		LatencyMs:    int64(latencyMs),
		TTFTMs:       ttftMs,
		CacheHit:     cacheHit,
		TPS:          tps,
	}

	// 1. Update service stats with comprehensive metrics
	s.updateServiceStats(rule, provider, model, metrics)

	// 2. Record to OTel with comprehensive usage data
	if s.tokenTracker != nil {
		s.tokenTracker.RecordUsage(c.Request.Context(), tracker.UsageOptions{
			Provider:         provider.Name,
			ProviderUUID:     provider.UUID,
			Model:            model,
			RequestModel:     requestModel,
			RuleUUID:         rule.UUID,
			Scenario:         scenario,
			InputTokens:      usage.InputTokens,
			OutputTokens:     usage.OutputTokens,
			CacheInputTokens: usage.CacheInputTokens,
			SystemTokens:     usage.SystemTokens,
			Streamed:         streamed,
			Status:           status,
			ErrorCode:        errorCode,
			LatencyMs:        latencyMs,
		})
	}

	// 3. Record detailed usage with comprehensive token data
	s.recordDetailedUsageWithTokenUsage(c, rule, provider, model, requestModel, scenario, usage, streamed, status, errorCode, latencyMs)

	// 4. Report to health monitor for service health tracking
	s.reportHealthStatus(provider, model, err, errorCode)

	// 5. Enterprise key-level 429 alerting hook (best-effort).
	if err != nil && isRateLimitError(err) && strings.TrimSpace(c.GetString("enterprise_user_id")) != "" {
		_ = reportEnterpriseRateLimitEvent(
			c.Request.Context(),
			c.GetString("enterprise_key_prefix"),
			provider.UUID,
			scenario,
			c.GetString("enterprise_user_id"),
		)
	}
}

// sanitizeErrorCode extracts a safe error code from an error.
func sanitizeErrorCode(err error) string {
	if err == nil {
		return ""
	}
	// Use error type name as code, avoid exposing sensitive info
	return err.Error()
}

func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "ratelimit")
}

// recordDetailedUsage writes a detailed usage record to the database.
// This maintains the detailed analytics tracking for the dashboard.
func (s *Server) recordDetailedUsage(c *gin.Context, rule *typ.Rule, provider *typ.Provider, model, requestModel, scenario string, inputTokens, outputTokens int, streamed bool, status, errorCode string, latencyMs int) {
	if s.config == nil {
		return
	}

	sm := s.config.StoreManager()
	if sm == nil {
		return
	}
	usageStore := sm.Usage()
	if usageStore == nil {
		return
	}

	record := &db.UsageRecord{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		Model:        model,
		Scenario:     scenario,
		RequestModel: requestModel,
		UserID:       c.GetString("enterprise_user_id"),
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

// recordDetailedUsageWithTokenUsage writes a detailed usage record with comprehensive token data.
func (s *Server) recordDetailedUsageWithTokenUsage(c *gin.Context, rule *typ.Rule, provider *typ.Provider, model, requestModel, scenario string, usage *protocol.TokenUsage, streamed bool, status, errorCode string, latencyMs int) {
	if s.config == nil || usage == nil {
		return
	}

	sm := s.config.StoreManager()
	if sm == nil {
		return
	}
	usageStore := sm.Usage()
	if usageStore == nil {
		return
	}

	record := &db.UsageRecord{
		ProviderUUID:     provider.UUID,
		ProviderName:     provider.Name,
		Model:            model,
		Scenario:         scenario,
		RequestModel:     requestModel,
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		CacheInputTokens: usage.CacheInputTokens,
		SystemTokens:     usage.SystemTokens,
		TotalTokens:      usage.TotalTokens(),
		Status:           status,
		ErrorCode:        errorCode,
		LatencyMs:        latencyMs,
		Streamed:         streamed,
	}

	if rule != nil {
		record.RuleUUID = rule.UUID
	}

	_ = usageStore.RecordUsage(record)
}

// updateServiceStats updates the service-level statistics for load balancing.
// This function records comprehensive metrics including tokens, latency, TTFT, cache, and TPS.
func (s *Server) updateServiceStats(rule *typ.Rule, provider *typ.Provider, model string, metrics MetricsData) {
	if rule == nil || provider == nil || s.config == nil {
		return
	}

	// Find the matching service in the rule and update its stats
	for i := range rule.Services {
		service := rule.Services[i]
		if service.Active && service.Provider == provider.UUID && service.Model == model {
			// Record basic token usage (also updates cost tokens internally)
			service.RecordUsage(metrics.InputTokens, metrics.OutputTokens)

			// Record latency metrics (always available)
			if metrics.LatencyMs > 0 {
				service.Stats.RecordLatency(metrics.LatencyMs, 100)
			}

			// Record TTFT if available (streaming requests)
			if metrics.TTFTMs > 0 {
				service.Stats.RecordTTFT(metrics.TTFTMs, 100)
			}

			// Record cache hit/miss
			service.Stats.RecordCacheHit(metrics.CacheHit)

			// Record TPS if available (streaming requests)
			if metrics.TPS > 0 {
				service.Stats.RecordTokenSpeed(metrics.TPS, 100)
			}

			// Persist to stats store
			sm := s.config.StoreManager()
			if sm != nil {
				if statsStore := sm.Stats(); statsStore != nil {
					_ = statsStore.UpdateFromService(service)
				}
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
