package usage

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/data/db"
)

// Handler provides REST endpoints for usage statistics
type Handler struct {
	usageStore *db.UsageStore
}

// NewHandler creates a new usage handler
func NewHandler(usageStore *db.UsageStore) *Handler {
	return &Handler{
		usageStore: usageStore,
	}
}

// GetStats returns aggregated usage statistics
func (h *Handler) GetStats(c *gin.Context) {
	if h.usageStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Usage store not available"})
		return
	}

	// Parse query parameters
	query := db.UsageStatsQuery{
		GroupBy:   c.DefaultQuery("group_by", "model"),
		Limit:     parseIntQuery(c, "limit", 100),
		SortBy:    c.DefaultQuery("sort_by", "total_tokens"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
		StartTime: parseTimeQuery(c, "start_time", time.Now().Add(-24*time.Hour)),
		EndTime:   parseTimeQuery(c, "end_time", time.Now()),
		Provider:  c.Query("provider"),
		Model:     c.Query("model"),
		Scenario:  c.Query("scenario"),
		RuleUUID:  c.Query("rule_uuid"),
		UserID:    c.Query("user_id"),
		Status:    c.Query("status"),
	}

	// Validate limit
	if query.Limit <= 0 || query.Limit > 1000 {
		query.Limit = 100
	}

	stats, err := h.usageStore.GetAggregatedStats(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Convert db.AggregatedStat to usage.AggregatedStat
	data := make([]AggregatedStat, len(stats))
	for i, s := range stats {
		data[i] = AggregatedStat(s)
	}

	response := UsageStatsResponse{
		Meta: UsageStatsMeta{
			StartTime:  query.StartTime.Format(time.RFC3339),
			EndTime:    query.EndTime.Format(time.RFC3339),
			GroupBy:    query.GroupBy,
			TotalCount: len(data),
		},
		Data: data,
	}

	c.JSON(http.StatusOK, response)
}

// GetTimeSeries returns time-series data for usage
func (h *Handler) GetTimeSeries(c *gin.Context) {
	if h.usageStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Usage store not available"})
		return
	}

	// Parse query parameters
	interval := c.DefaultQuery("interval", "hour")
	startTime := parseTimeQuery(c, "start_time", time.Now().Add(-24*time.Hour))
	endTime := parseTimeQuery(c, "end_time", time.Now())

	// Build filters
	filters := make(map[string]string)
	if provider := c.Query("provider"); provider != "" {
		filters["provider_uuid"] = provider
	}
	if model := c.Query("model"); model != "" {
		filters["model"] = model
	}
	if scenario := c.Query("scenario"); scenario != "" {
		filters["scenario"] = scenario
	}
	if userID := c.Query("user_id"); userID != "" {
		filters["user_id"] = userID
	}

	data, err := h.usageStore.GetTimeSeries(interval, startTime, endTime, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Convert db.TimeSeriesData to usage.TimeSeriesData
	result := make([]TimeSeriesData, len(data))
	for i, d := range data {
		result[i] = TimeSeriesData{
			Timestamp:    d.Timestamp,
			RequestCount: d.RequestCount,
			TotalTokens:  d.TotalTokens,
			InputTokens:  d.InputTokens,
			OutputTokens: d.OutputTokens,
			ErrorCount:   d.ErrorCount,
			AvgLatencyMs: d.AvgLatencyMs,
		}
	}

	response := TimeSeriesResponse{
		Meta: TimeSeriesMeta{
			Interval:  interval,
			StartTime: startTime.Format(time.RFC3339),
			EndTime:   endTime.Format(time.RFC3339),
		},
		Data: result,
	}

	c.JSON(http.StatusOK, response)
}

// GetRecords returns individual usage records
func (h *Handler) GetRecords(c *gin.Context) {
	if h.usageStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Usage store not available"})
		return
	}

	// Parse query parameters
	startTime := parseTimeQuery(c, "start_time", time.Now().Add(-1*time.Hour))
	endTime := parseTimeQuery(c, "end_time", time.Now())
	limit := parseIntQuery(c, "limit", 50)
	offset := parseIntQuery(c, "offset", 0)

	// Validate limit
	if limit <= 0 || limit > 1000 {
		limit = 50
	}

	// Build filters
	filters := make(map[string]string)
	if provider := c.Query("provider"); provider != "" {
		filters["provider_uuid"] = provider
	}
	if model := c.Query("model"); model != "" {
		filters["model"] = model
	}
	if scenario := c.Query("scenario"); scenario != "" {
		filters["scenario"] = scenario
	}
	if status := c.Query("status"); status != "" {
		filters["status"] = status
	}
	if userID := c.Query("user_id"); userID != "" {
		filters["user_id"] = userID
	}

	records, total, err := h.usageStore.GetRecords(startTime, endTime, filters, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Convert db.UsageRecord to usage.UsageRecordResponse
	data := make([]UsageRecordResponse, len(records))
	for i, r := range records {
		data[i] = UsageRecordResponse{
			ID:           r.ID,
			ProviderUUID: r.ProviderUUID,
			ProviderName: r.ProviderName,
			Model:        r.Model,
			Scenario:     r.Scenario,
			RuleUUID:     r.RuleUUID,
			UserID:       r.UserID,
			RequestModel: r.RequestModel,
			Timestamp:    r.Timestamp.Format(time.RFC3339),
			InputTokens:  r.InputTokens,
			OutputTokens: r.OutputTokens,
			TotalTokens:  r.TotalTokens,
			Status:       r.Status,
			ErrorCode:    r.ErrorCode,
			LatencyMs:    r.LatencyMs,
			Streamed:     r.Streamed,
		}
	}

	response := UsageRecordsResponse{
		Meta: UsageRecordsMeta{
			Total:  int(total),
			Limit:  limit,
			Offset: offset,
		},
		Data: data,
	}

	c.JSON(http.StatusOK, response)
}

// DeleteOldRecords deletes usage records older than the specified date
func (h *Handler) DeleteOldRecords(c *gin.Context) {
	if h.usageStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Usage store not available"})
		return
	}

	var req DeleteOldRecordsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cutoffDate := time.Now().AddDate(0, 0, -req.OlderThanDays)
	deleted, err := h.usageStore.DeleteOlderThan(cutoffDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := DeleteOldRecordsResponse{
		Message:      "Records deleted successfully",
		DeletedCount: deleted,
		CutoffDate:   cutoffDate.Format(time.RFC3339),
	}

	c.JSON(http.StatusOK, response)
}

// Helper functions

func parseIntQuery(c *gin.Context, key string, defaultValue int) int {
	if value := c.Query(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func parseTimeQuery(c *gin.Context, key string, defaultValue time.Time) time.Time {
	if value := c.Query(key); value != "" {
		if timeValue, err := time.Parse(time.RFC3339, value); err == nil {
			return timeValue
		}
		// Try alternative formats
		layouts := []string{
			"2006-01-02T15:04:05Z",
			"2006-01-02T15:04:05",
			"2006-01-02 15:04:05",
			"2006-01-02",
		}
		for _, layout := range layouts {
			if timeValue, err := time.Parse(layout, value); err == nil {
				return timeValue
			}
		}
	}
	return defaultValue
}

func intPtr(v int) *int {
	return &v
}
