package usage

// =============================================
// Usage API Models
// =============================================

// UsageStatsQuery represents query parameters for usage statistics
type UsageStatsQuery struct {
	GroupBy   string `json:"group_by" form:"group_by" description:"Aggregation level: model, provider, scenario, rule, user, daily, hourly" example:"model"`
	StartTime string `json:"start_time" form:"start_time" description:"ISO 8601 start time" example:"2025-01-10T00:00:00Z"`
	EndTime   string `json:"end_time" form:"end_time" description:"ISO 8601 end time" example:"2025-01-11T00:00:00Z"`
	Provider  string `json:"provider" form:"provider" description:"Filter by provider UUID"`
	Model     string `json:"model" form:"model" description:"Filter by model name"`
	Scenario  string `json:"scenario" form:"scenario" description:"Filter by scenario"`
	RuleUUID  string `json:"rule_uuid" form:"rule_uuid" description:"Filter by rule UUID"`
	UserID    string `json:"user_id" form:"user_id" description:"Filter by enterprise user ID"`
	Status    string `json:"status" form:"status" description:"Filter by status: success, error, partial" example:"success"`
	Limit     int    `json:"limit" form:"limit" description:"Max results to return" example:"100"`
	SortBy    string `json:"sort_by" form:"sort_by" description:"Sort field: total_tokens, request_count, avg_latency" example:"total_tokens"`
	SortOrder string `json:"sort_order" form:"sort_order" description:"asc or desc" example:"desc"`
}

// AggregatedStat represents aggregated usage statistics
type AggregatedStat struct {
	Key              string  `json:"key" example:"gpt-4"`
	ProviderUUID     string  `json:"provider_uuid,omitempty" example:"uuid-123"`
	ProviderName     string  `json:"provider_name,omitempty" example:"openai"`
	Model            string  `json:"model,omitempty" example:"gpt-4"`
	Scenario         string  `json:"scenario,omitempty" example:"openai"`
	UserID           string  `json:"user_id,omitempty" example:"usr_123"`
	RequestCount     int64   `json:"request_count" example:"5420"`
	TotalTokens      int64   `json:"total_tokens" example:"2140000"`
	InputTokens      int64   `json:"total_input_tokens" example:"1250000"`
	OutputTokens     int64   `json:"total_output_tokens" example:"890000"`
	AvgInputTokens   float64 `json:"avg_input_tokens" example:"230.6"`
	AvgOutputTokens  float64 `json:"avg_output_tokens" example:"164.2"`
	AvgLatencyMs     float64 `json:"avg_latency_ms" example:"1250"`
	ErrorCount       int64   `json:"error_count" example:"12"`
	ErrorRate        float64 `json:"error_rate" example:"0.0022"`
	StreamedCount    int64   `json:"streamed_count" example:"4800"`
	StreamedRate     float64 `json:"streamed_rate" example:"0.885"`
	CacheInputTokens int64   `json:"cache_input_tokens" example:"500000"`
}

// UsageStatsResponse represents the response for usage statistics
type UsageStatsResponse struct {
	Meta UsageStatsMeta   `json:"meta"`
	Data []AggregatedStat `json:"data"`
}

// UsageStatsMeta represents metadata for usage statistics response
type UsageStatsMeta struct {
	StartTime  string `json:"start_time" example:"2025-01-10T00:00:00Z"`
	EndTime    string `json:"end_time" example:"2025-01-11T00:00:00Z"`
	GroupBy    string `json:"group_by" example:"model"`
	TotalCount int    `json:"total_count" example:"10"`
}

// TimeSeriesQuery represents query parameters for time-series data
type TimeSeriesQuery struct {
	Interval  string `json:"interval" form:"interval" description:"Time bucket: minute, hour, day, week" example:"hour"`
	StartTime string `json:"start_time" form:"start_time" description:"ISO 8601 start time" example:"2025-01-10T00:00:00Z"`
	EndTime   string `json:"end_time" form:"end_time" description:"ISO 8601 end time" example:"2025-01-11T00:00:00Z"`
	Provider  string `json:"provider" form:"provider" description:"Filter by provider UUID"`
	Model     string `json:"model" form:"model" description:"Filter by model name"`
	Scenario  string `json:"scenario" form:"scenario" description:"Filter by scenario"`
}

// TimeSeriesData represents a single time bucket in time series data
type TimeSeriesData struct {
	Timestamp        string  `json:"timestamp" example:"2025-01-10T00:00:00Z"`
	RequestCount     int64   `json:"request_count" example:"245"`
	TotalTokens      int64   `json:"total_tokens" example:"52000"`
	InputTokens      int64   `json:"input_tokens" example:"32000"`
	OutputTokens     int64   `json:"output_tokens" example:"20000"`
	CacheInputTokens int64   `json:"cache_input_tokens" example:"10000"`
	ErrorCount       int64   `json:"error_count" example:"0"`
	AvgLatencyMs     float64 `json:"avg_latency_ms" example:"1100"`
}

// TimeSeriesResponse represents the response for time-series data
type TimeSeriesResponse struct {
	Meta TimeSeriesMeta   `json:"meta"`
	Data []TimeSeriesData `json:"data"`
}

// TimeSeriesMeta represents metadata for time-series response
type TimeSeriesMeta struct {
	Interval  string `json:"interval" example:"hour"`
	StartTime string `json:"start_time" example:"2025-01-10T00:00:00Z"`
	EndTime   string `json:"end_time" example:"2025-01-11T00:00:00Z"`
}

// UsageRecordsQuery represents query parameters for usage records
type UsageRecordsQuery struct {
	StartTime string `json:"start_time" form:"start_time" description:"ISO 8601 start time" example:"2025-01-10T00:00:00Z"`
	EndTime   string `json:"end_time" form:"end_time" description:"ISO 8601 end time" example:"2025-01-11T00:00:00Z"`
	Provider  string `json:"provider" form:"provider" description:"Filter by provider UUID"`
	Model     string `json:"model" form:"model" description:"Filter by model name"`
	Scenario  string `json:"scenario" form:"scenario" description:"Filter by scenario"`
	Status    string `json:"status" form:"status" description:"Filter by status"`
	Limit     int    `json:"limit" form:"limit" description:"Max results (max 1000)" example:"50"`
	Offset    int    `json:"offset" form:"offset" description:"Pagination offset" example:"0"`
}

// UsageRecordResponse represents a single usage record
type UsageRecordResponse struct {
	ID               uint   `json:"id" example:"1"`
	ProviderUUID     string `json:"provider_uuid" example:"uuid-123"`
	ProviderName     string `json:"provider_name" example:"openai"`
	Model            string `json:"model" example:"gpt-4"`
	Scenario         string `json:"scenario" example:"openai"`
	RuleUUID         string `json:"rule_uuid,omitempty" example:"rule-uuid"`
	UserID           string `json:"user_id,omitempty" example:"usr_123"`
	RequestModel     string `json:"request_model,omitempty" example:"gpt-4"`
	Timestamp        string `json:"timestamp" example:"2025-01-10T12:00:00Z"`
	InputTokens      int    `json:"input_tokens" example:"1000"`
	OutputTokens     int    `json:"output_tokens" example:"500"`
	TotalTokens      int    `json:"total_tokens" example:"1500"`
	CacheInputTokens int    `json:"cache_input_tokens" example:"2000"`
	Status           string `json:"status" example:"success"`
	ErrorCode        string `json:"error_code,omitempty"`
	LatencyMs        int    `json:"latency_ms" example:"1200"`
	Streamed         bool   `json:"streamed" example:"true"`
}

// UsageRecordsResponse represents the response for usage records
type UsageRecordsResponse struct {
	Meta UsageRecordsMeta      `json:"meta"`
	Data []UsageRecordResponse `json:"data"`
}

// UsageRecordsMeta represents metadata for usage records response
type UsageRecordsMeta struct {
	Total  int `json:"total" example:"1000"`
	Limit  int `json:"limit" example:"50"`
	Offset int `json:"offset" example:"0"`
}

// DeleteOldRecordsRequest represents the request to delete old usage records
type DeleteOldRecordsRequest struct {
	OlderThanDays int `json:"older_than_days" binding:"required,min=1" description:"Delete records older than this many days" example:"90"`
}

// DeleteOldRecordsResponse represents the response for deleting old records
type DeleteOldRecordsResponse struct {
	Message      string `json:"message" example:"Records deleted successfully"`
	DeletedCount int64  `json:"deleted_count" example:"1500"`
	CutoffDate   string `json:"cutoff_date" example:"2024-10-13T00:00:00Z"`
}
