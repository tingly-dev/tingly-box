package protocolserver

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/otel/tracker"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
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
	TPS          float64 // Per-request output TPS after TTFT (0 when unavailable)
}

// GetUserIDFromContext extracts the user ID from gin context.
// Priority: user_id (from JWT API token or global auth) > enterprise_user_id (from enterprise JWT) > ""
// This ensures that JWT API token authentication takes precedence for user tracking.
func GetUserIDFromContext(c *gin.Context) string {
	// First, try user_id from JWT API token authentication or global auth
	if userID := c.GetString(constant.CtxKeyUserID); userID != "" {
		return userID
	}
	// Fall back to enterprise_user_id from enterprise context JWT
	if enterpriseUserID := c.GetString(constant.CtxKeyEnterpriseUserID); enterpriseUserID != "" {
		return enterpriseUserID
	}
	return ""
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
func (ph *ProtocolHandler) trackUsageFromContext(c *gin.Context, inputTokens, outputTokens int, err error) {
	rule, provider, model, requestModel, scenario, streamed, startTime := GetTrackingContext(c)

	if rule == nil || provider == nil || model == "" {
		return
	}

	latencyMs := CalculateLatencyFromStart(startTime)

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

	// 1. Update in-memory service stats with comprehensive metrics
	service := ph.updateServiceStats(rule, provider, model, metrics)

	// 2. Record to OTel (primary path for metrics)
	if ph.deps.TokenTracker != nil {
		userTier := ""
		if strings.TrimSpace(c.GetString(constant.CtxKeyEnterpriseUserID)) != "" {
			userTier = "enterprise"
		}
		// Metric attributes must stay low-cardinality: pass the bounded error
		// class, never the raw error message (see classifyErrorCode).
		ph.deps.TokenTracker.RecordUsage(c.Request.Context(), tracker.UsageOptions{
			Operation:    OperationFromContext(c),
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
			ErrorCode:    classifyErrorCode(err),
			LatencyMs:    latencyMs,
			UserTier:     userTier,
		})
	}

	// 3. Mirror token usage onto the request span (write-only projection;
	// the span never feeds data back — see .design/otel.md §7.1)
	ph.setTokenUsageOnSpan(c, inputTokens, outputTokens)

	// 4. Persist service stats and detailed usage together in one transaction
	usageRecord := ph.recordDetailedUsage(c, rule, provider, model, requestModel, scenario, inputTokens, outputTokens, streamed, status, errorCode, latencyMs)
	ph.persistRequestOutcome(service, usageRecord)

	// 5. Report to health monitor for service health tracking
	ph.ReportHealthStatus(provider, model, err, errorCode)

	// 6. Enterprise key-level 429 alerting hook (best-effort).
	if err != nil && isRateLimitError(err) && strings.TrimSpace(c.GetString(constant.CtxKeyEnterpriseUserID)) != "" {
		_ = reportEnterpriseRateLimitEvent(
			c.Request.Context(),
			c.GetString(constant.CtxKeyEnterpriseKeyPrefix),
			provider.UUID,
			scenario,
			c.GetString(constant.CtxKeyEnterpriseUserID),
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
func (ph *ProtocolHandler) trackUsageWithTokenUsage(c *gin.Context, usage *protocol.TokenUsage, err error) {
	rule, provider, model, requestModel, scenario, streamed, startTime := GetTrackingContext(c)

	logrus.WithFields(logrus.Fields{
		"has_rule":     rule != nil,
		"has_provider": provider != nil,
		"has_model":    model != "",
		"has_usage":    usage != nil,
		"has_error":    err != nil,
		"model":        model,
	}).Trace("[trackUsage] trackUsageWithTokenUsage called")

	if rule == nil || provider == nil || model == "" || usage == nil {
		return
	}

	latencyMs := CalculateLatencyFromStart(startTime)

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
		"provider":         provider.Name,
		"model":            model,
		"scenario":         scenario,
		"input_tokens":     usage.InputTokens,
		"output_tokens":    usage.OutputTokens,
		"cache_tokens":     usage.CacheReadTokens,
		"cache_write":      usage.CacheWriteTokens,
		"reasoning_tokens": usage.ReasoningTokens,
		"system_tokens":    usage.SystemTokens,
		"total_tokens":     usage.TotalTokens(),
		"status":           status,
		"streamed":         streamed,
		"latency_ms":       latencyMs,
	}).Debug("trackUsage: token usage recorded")

	// Detect cache hit from usage data and set in context
	cacheHit := DetectCacheHit(usage)
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

	// 1. Update in-memory service stats with comprehensive metrics
	service := ph.updateServiceStats(rule, provider, model, metrics)

	// 2. Record to OTel with comprehensive usage data
	if ph.deps.TokenTracker != nil {
		// Metric attributes must stay low-cardinality: pass the bounded error
		// class, never the raw error message (see classifyErrorCode).
		ph.deps.TokenTracker.RecordUsage(c.Request.Context(), tracker.UsageOptions{
			Operation:        OperationFromContext(c),
			Provider:         provider.Name,
			ProviderUUID:     provider.UUID,
			Model:            model,
			RequestModel:     requestModel,
			RuleUUID:         rule.UUID,
			Scenario:         scenario,
			InputTokens:      usage.InputTokens,
			OutputTokens:     usage.OutputTokens,
			CacheReadTokens:  usage.CacheReadTokens,
			CacheWriteTokens: usage.CacheWriteTokens,
			SystemTokens:     usage.SystemTokens,
			Streamed:         streamed,
			Status:           status,
			ErrorCode:        classifyErrorCode(err),
			LatencyMs:        latencyMs,
		})
	}

	// 3. Mirror token usage onto the request span (write-only projection;
	// the span never feeds data back — see .design/otel.md §7.1)
	ph.setTokenUsageOnSpan(c, usage.InputTokens, usage.OutputTokens)

	// 4. Persist service stats and detailed usage together in one transaction
	usageRecord := ph.recordDetailedUsageWithTokenUsage(c, rule, provider, model, requestModel, scenario, usage, streamed, status, errorCode, latencyMs)
	ph.persistRequestOutcome(service, usageRecord)

	// 5. Report to health monitor for service health tracking
	ph.ReportHealthStatus(provider, model, err, errorCode)

	// 6. Enterprise key-level 429 alerting hook (best-effort).
	if err != nil && isRateLimitError(err) && strings.TrimSpace(c.GetString(constant.CtxKeyEnterpriseUserID)) != "" {
		_ = reportEnterpriseRateLimitEvent(
			c.Request.Context(),
			c.GetString(constant.CtxKeyEnterpriseKeyPrefix),
			provider.UUID,
			scenario,
			c.GetString(constant.CtxKeyEnterpriseUserID),
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

// classifyErrorCode maps an error to a small, bounded set of class labels for
// use as an OTel metric attribute. Metric attributes must be low-cardinality:
// every distinct value permanently allocates a new data point per instrument
// in the cumulative metrics SDK, so passing raw err.Error() (which can embed
// upstream response bodies, IDs and URLs) leaks memory one timeseries at a
// time (#1255). The detailed message still goes to the usage DB record.
func classifyErrorCode(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "client_disconnected"
	}
	errStr := strings.ToLower(err.Error())
	switch {
	// Same signals as isRateLimitError, tested against the already-lowered
	// string so a large error payload is not lowercased twice.
	case strings.Contains(errStr, "429") || strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "ratelimit") || strings.Contains(errStr, "1302"):
		return "rate_limit"
	case strings.Contains(errStr, "401") || strings.Contains(errStr, "unauthorized"):
		return "auth_401"
	case strings.Contains(errStr, "403") || strings.Contains(errStr, "forbidden"):
		return "auth_403"
	case strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline"):
		return "timeout"
	case strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "no such host") || strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "eof"):
		return "connection"
	case strings.Contains(errStr, "404") || strings.Contains(errStr, "not found"):
		return "not_found"
	case strings.Contains(errStr, "400") || strings.Contains(errStr, "invalid_request") ||
		strings.Contains(errStr, "bad request"):
		return "bad_request"
	case strings.Contains(errStr, "overloaded") || strings.Contains(errStr, "529") ||
		strings.Contains(errStr, "500") || strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") || strings.Contains(errStr, "504"):
		return "upstream_5xx"
	case strings.Contains(errStr, "stream"):
		return "stream_error"
	default:
		return "other"
	}
}

func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "ratelimit") ||
		strings.Contains(errStr, "1302")
}

// recordDetailedUsage builds the detailed usage record for the analytics
// dashboard; persistRequestOutcome saves it together with the matching
// StatsStore update.
func (ph *ProtocolHandler) recordDetailedUsage(c *gin.Context, rule *typ.Rule, provider *typ.Provider, model, requestModel, scenario string, inputTokens, outputTokens int, streamed bool, status, errorCode string, latencyMs int) *db.UsageRecord {
	ttftMs := CalculateTTFT(c)

	record := &db.UsageRecord{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		Model:        model,
		Scenario:     scenario,
		RequestModel: requestModel,
		UserID:       GetUserIDFromContext(c), // Uses user_id or enterprise_user_id
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  inputTokens + outputTokens,
		Status:       status,
		ErrorCode:    errorCode,
		LatencyMs:    latencyMs,
		TTFTMs:       int(ttftMs),
		Streamed:     streamed,
		TraceID:      c.GetString(constant.CtxKeyTraceID),
	}

	if rule != nil {
		record.RuleUUID = rule.UUID
	}

	return record
}

// recordDetailedUsageWithTokenUsage is recordDetailedUsage with comprehensive
// token data (cache/system tokens).
func (ph *ProtocolHandler) recordDetailedUsageWithTokenUsage(c *gin.Context, rule *typ.Rule, provider *typ.Provider, model, requestModel, scenario string, usage *protocol.TokenUsage, streamed bool, status, errorCode string, latencyMs int) *db.UsageRecord {
	if usage == nil {
		return nil
	}

	ttftMs := CalculateTTFT(c)

	record := &db.UsageRecord{
		ProviderUUID:     provider.UUID,
		ProviderName:     provider.Name,
		Model:            model,
		Scenario:         scenario,
		RequestModel:     requestModel,
		UserID:           GetUserIDFromContext(c), // Uses user_id or enterprise_user_id
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		CacheReadTokens:  usage.CacheReadTokens,
		CacheWriteTokens: usage.CacheWriteTokens,
		ReasoningTokens:  usage.ReasoningTokens,
		SystemTokens:     usage.SystemTokens,
		TotalTokens:      usage.TotalTokens(),
		Status:           status,
		ErrorCode:        errorCode,
		LatencyMs:        latencyMs,
		TTFTMs:           int(ttftMs),
		Streamed:         streamed,
		TraceID:          c.GetString(constant.CtxKeyTraceID),
	}

	if rule != nil {
		record.RuleUUID = rule.UUID
	}

	return record
}

// updateServiceStats updates the in-memory service-level stats (tokens,
// latency, TTFT, cache, TPS) and returns the matched service for
// persistRequestOutcome to save.
func (ph *ProtocolHandler) updateServiceStats(rule *typ.Rule, provider *typ.Provider, model string, metrics MetricsData) *loadbalance.Service {
	if rule == nil || provider == nil {
		return nil
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

			return service
		}
	}
	return nil
}

// persistRequestOutcome saves service and usage (either may be nil) via
// db.RecordRequestOutcome, which commits both in a single transaction.
func (ph *ProtocolHandler) persistRequestOutcome(service *loadbalance.Service, usage *db.UsageRecord) {
	if ph.deps.Config == nil {
		return
	}
	sm := ph.deps.Config.StoreManager()
	if sm == nil {
		return
	}
	_ = db.RecordRequestOutcome(sm.Stats(), sm.Usage(), service, usage)
}

// reportHealthStatus reports the health status of a service based on request outcome.
// It uses the health monitor to track service health for load balancing decisions.
func (ph *ProtocolHandler) ReportHealthStatus(provider *typ.Provider, model string, err error, errorCode string) {
	if ph.deps.HealthMonitor == nil {
		logrus.Warn("[health] healthMonitor is nil - cannot report health status")
		return
	}
	if provider == nil || model == "" {
		logrus.Warn("[health] provider or model is empty - cannot report health status")
		return
	}

	serviceID := loadbalance.FormatServiceID(provider.UUID, model)

	logrus.WithFields(logrus.Fields{
		"provider":   provider.Name,
		"model":      model,
		"service_id": serviceID,
		"error":      err != nil,
		"errorCode":  errorCode,
	}).Debug("[health] Reporting health status")

	if err == nil {
		// Success - report to health monitor
		ph.deps.HealthMonitor.ReportSuccess(serviceID)
		return
	}

	// Error - classify and report appropriately
	errStr := err.Error()

	// Check for rate limit (429, 1302)
	if strings.Contains(errStr, "429") || strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "RateLimit") ||
		strings.Contains(errStr, "1302") || strings.Contains(errStr, "\"code\":\"1302\"") {
		logrus.WithFields(logrus.Fields{
			"service_id": serviceID,
			"provider":   provider.Name,
			"model":      model,
			"error":      errStr,
		}).Warn("[health] Rate limit detected - marking service unhealthy")
		ph.deps.HealthMonitor.ReportRateLimit(serviceID)
		return
	}

	// Check for auth errors (401/403)
	if strings.Contains(errStr, "401") || strings.Contains(errStr, "403") ||
		strings.Contains(errStr, "unauthorized") || strings.Contains(errStr, "forbidden") {
		// Try to determine if it's 401 or 403
		if strings.Contains(errStr, "401") {
			ph.deps.HealthMonitor.ReportAuthError(serviceID, 401)
		} else {
			ph.deps.HealthMonitor.ReportAuthError(serviceID, 403)
		}
		return
	}

	// Generic failures (5xx, timeouts, connection errors) deliberately do NOT
	// feed the health monitor: the circuit breaker owns that signal, fed by
	// the failover loop and rule-scoped so one rule's failing traffic cannot
	// evict the service for every other rule. The health monitor tracks only
	// the status classes with distinct semantics: 429 rate-limit windows and
	// 401/403 auth failures.
	logrus.WithFields(logrus.Fields{
		"service_id": serviceID,
		"error":      errStr,
	}).Debug("[health] generic failure left to the circuit breaker")
}
