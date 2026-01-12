package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"tingly-box/internal/db"
	"tingly-box/internal/server/config"
	"tingly-box/pkg/swagger"
)

// UsageAPI provides REST endpoints for usage statistics
type UsageAPI struct {
	config     *config.Config
	usageStore *db.UsageStore
}

// NewUsageAPI creates a new usage API
func NewUsageAPI(cfg *config.Config) *UsageAPI {
	return &UsageAPI{
		config:     cfg,
		usageStore: cfg.GetUsageStore(),
	}
}

// RegisterUsageRoutes registers the usage API routes with swagger documentation
func (s *Server) RegisterUsageRoutes(manager *swagger.RouteManager) {
	// Create authenticated API group for usage
	usageAPI := NewUsageAPI(s.config)

	apiV1 := manager.NewGroup("api", "v1", "")
	apiV1.Router.Use(s.authMW.UserAuthMiddleware())

	// GET /api/v1/usage/stats - Get aggregated usage statistics
	apiV1.GET("/usage/stats", usageAPI.GetStats,
		swagger.WithTags("usage"),
		swagger.WithDescription("Returns aggregated usage statistics with flexible grouping and filtering"),
		swagger.WithQueryConfig("group_by", swagger.QueryParamConfig{
			Name:        "group_by",
			Type:        "string",
			Required:    false,
			Description: "Aggregation level: model, provider, scenario, rule, daily, hourly",
			Default:     "model",
			Enum:        []interface{}{"model", "provider", "scenario", "rule", "daily", "hourly"},
		}),
		swagger.WithQueryConfig("start_time", swagger.QueryParamConfig{
			Name:        "start_time",
			Type:        "string",
			Required:    false,
			Description: "ISO 8601 start time",
		}),
		swagger.WithQueryConfig("end_time", swagger.QueryParamConfig{
			Name:        "end_time",
			Type:        "string",
			Required:    false,
			Description: "ISO 8601 end time",
		}),
		swagger.WithQueryConfig("provider", swagger.QueryParamConfig{
			Name:        "provider",
			Type:        "string",
			Required:    false,
			Description: "Filter by provider UUID",
		}),
		swagger.WithQueryConfig("model", swagger.QueryParamConfig{
			Name:        "model",
			Type:        "string",
			Required:    false,
			Description: "Filter by model name",
		}),
		swagger.WithQueryConfig("scenario", swagger.QueryParamConfig{
			Name:        "scenario",
			Type:        "string",
			Required:    false,
			Description: "Filter by scenario",
		}),
		swagger.WithQueryConfig("rule_uuid", swagger.QueryParamConfig{
			Name:        "rule_uuid",
			Type:        "string",
			Required:    false,
			Description: "Filter by rule UUID",
		}),
		swagger.WithQueryConfig("status", swagger.QueryParamConfig{
			Name:        "status",
			Type:        "string",
			Required:    false,
			Description: "Filter by status: success, error, partial",
			Default:     "success",
			Enum:        []interface{}{"success", "error", "partial"},
		}),
		swagger.WithQueryConfig("limit", swagger.QueryParamConfig{
			Name:        "limit",
			Type:        "integer",
			Required:    false,
			Description: "Max results to return",
			Default:     100,
			Minimum:     intPtr(1),
			Maximum:     intPtr(1000),
		}),
		swagger.WithQueryConfig("sort_by", swagger.QueryParamConfig{
			Name:        "sort_by",
			Type:        "string",
			Required:    false,
			Description: "Sort field: total_tokens, request_count, avg_latency",
			Default:     "total_tokens",
			Enum:        []interface{}{"total_tokens", "request_count", "avg_latency"},
		}),
		swagger.WithQueryConfig("sort_order", swagger.QueryParamConfig{
			Name:        "sort_order",
			Type:        "string",
			Required:    false,
			Description: "asc or desc",
			Default:     "desc",
			Enum:        []interface{}{"asc", "desc"},
		}),
		swagger.WithResponseModel(UsageStatsResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 503, Message: "Usage store not available"},
		),
	)

	// GET /api/v1/usage/timeseries - Get time-series usage data
	apiV1.GET("/usage/timeseries", usageAPI.GetTimeSeries,
		swagger.WithTags("usage"),
		swagger.WithDescription("Returns time-series data for usage with configurable intervals"),
		swagger.WithQueryConfig("interval", swagger.QueryParamConfig{
			Name:        "interval",
			Type:        "string",
			Required:    false,
			Description: "Time bucket: minute, hour, day, week",
			Default:     "hour",
			Enum:        []interface{}{"minute", "hour", "day", "week"},
		}),
		swagger.WithQueryConfig("start_time", swagger.QueryParamConfig{
			Name:        "start_time",
			Type:        "string",
			Required:    false,
			Description: "ISO 8601 start time",
		}),
		swagger.WithQueryConfig("end_time", swagger.QueryParamConfig{
			Name:        "end_time",
			Type:        "string",
			Required:    false,
			Description: "ISO 8601 end time",
		}),
		swagger.WithQueryConfig("provider", swagger.QueryParamConfig{
			Name:        "provider",
			Type:        "string",
			Required:    false,
			Description: "Filter by provider UUID",
		}),
		swagger.WithQueryConfig("model", swagger.QueryParamConfig{
			Name:        "model",
			Type:        "string",
			Required:    false,
			Description: "Filter by model name",
		}),
		swagger.WithQueryConfig("scenario", swagger.QueryParamConfig{
			Name:        "scenario",
			Type:        "string",
			Required:    false,
			Description: "Filter by scenario",
		}),
		swagger.WithResponseModel(TimeSeriesResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 503, Message: "Usage store not available"},
		),
	)

	// GET /api/v1/usage/records - Get individual usage records
	apiV1.GET("/usage/records", usageAPI.GetRecords,
		swagger.WithTags("usage"),
		swagger.WithDescription("Returns individual usage records (for debugging/audit)"),
		swagger.WithQueryConfig("start_time", swagger.QueryParamConfig{
			Name:        "start_time",
			Type:        "string",
			Required:    false,
			Description: "ISO 8601 start time",
		}),
		swagger.WithQueryConfig("end_time", swagger.QueryParamConfig{
			Name:        "end_time",
			Type:        "string",
			Required:    false,
			Description: "ISO 8601 end time",
		}),
		swagger.WithQueryConfig("provider", swagger.QueryParamConfig{
			Name:        "provider",
			Type:        "string",
			Required:    false,
			Description: "Filter by provider UUID",
		}),
		swagger.WithQueryConfig("model", swagger.QueryParamConfig{
			Name:        "model",
			Type:        "string",
			Required:    false,
			Description: "Filter by model name",
		}),
		swagger.WithQueryConfig("scenario", swagger.QueryParamConfig{
			Name:        "scenario",
			Type:        "string",
			Required:    false,
			Description: "Filter by scenario",
		}),
		swagger.WithQueryConfig("status", swagger.QueryParamConfig{
			Name:        "status",
			Type:        "string",
			Required:    false,
			Description: "Filter by status",
		}),
		swagger.WithQueryConfig("limit", swagger.QueryParamConfig{
			Name:        "limit",
			Type:        "integer",
			Required:    false,
			Description: "Max results (max 1000)",
			Default:     50,
			Minimum:     intPtr(1),
			Maximum:     intPtr(1000),
		}),
		swagger.WithQueryConfig("offset", swagger.QueryParamConfig{
			Name:        "offset",
			Type:        "integer",
			Required:    false,
			Description: "Pagination offset",
			Default:     0,
			Minimum:     intPtr(0),
		}),
		swagger.WithResponseModel(UsageRecordsResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 503, Message: "Usage store not available"},
		),
	)

	// DELETE /api/v1/usage/records - Delete old usage records
	apiV1.DELETE("/usage/records", usageAPI.DeleteOldRecords,
		swagger.WithTags("usage"),
		swagger.WithDescription("Deletes usage records older than the specified number of days"),
		swagger.WithRequestModel(DeleteOldRecordsRequest{}),
		swagger.WithResponseModel(DeleteOldRecordsResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 400, Message: "Invalid request"},
			swagger.ErrorResponseConfig{Code: 503, Message: "Usage store not available"},
		),
	)
}

// GetStats returns aggregated usage statistics
func (api *UsageAPI) GetStats(c *gin.Context) {
	if api.usageStore == nil {
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
		Status:    c.Query("status"),
	}

	// Validate limit
	if query.Limit <= 0 || query.Limit > 1000 {
		query.Limit = 100
	}

	stats, err := api.usageStore.GetAggregatedStats(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Convert db.AggregatedStat to server.AggregatedStat
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
func (api *UsageAPI) GetTimeSeries(c *gin.Context) {
	if api.usageStore == nil {
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

	data, err := api.usageStore.GetTimeSeries(interval, startTime, endTime, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Convert db.TimeSeriesData to server.TimeSeriesData
	result := make([]TimeSeriesData, len(data))
	for i, d := range data {
		result[i] = TimeSeriesData(d)
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
func (api *UsageAPI) GetRecords(c *gin.Context) {
	if api.usageStore == nil {
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

	records, total, err := api.usageStore.GetRecords(startTime, endTime, filters, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Convert db.UsageRecord to server.UsageRecordResponse
	data := make([]UsageRecordResponse, len(records))
	for i, r := range records {
		data[i] = UsageRecordResponse{
			ID:           r.ID,
			ProviderUUID: r.ProviderUUID,
			ProviderName: r.ProviderName,
			Model:        r.Model,
			Scenario:     r.Scenario,
			RuleUUID:     r.RuleUUID,
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
func (api *UsageAPI) DeleteOldRecords(c *gin.Context) {
	if api.usageStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Usage store not available"})
		return
	}

	var req DeleteOldRecordsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cutoffDate := time.Now().AddDate(0, 0, -req.OlderThanDays)
	deleted, err := api.usageStore.DeleteOlderThan(cutoffDate)
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
