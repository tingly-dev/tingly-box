package db

import (
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"tingly-box/internal/constant"
)

// UsageRecord is the GORM model for persisting individual usage records
type UsageRecord struct {
	ID           uint      `gorm:"primaryKey;autoIncrement;column:id"`
	ProviderUUID string    `gorm:"column:provider_uuid;index:idx_provider_model;not null"`
	ProviderName string    `gorm:"column:provider_name;not null"`
	Model        string    `gorm:"column:model;index:idx_provider_model;not null"`
	Scenario     string    `gorm:"column:scenario;index:idx_scenario;not null"`
	RuleUUID     string    `gorm:"column:rule_uuid;index:idx_rule"`
	RequestModel string    `gorm:"column:request_model"`
	Timestamp    time.Time `gorm:"column:timestamp;index:idx_timestamp;index:idx_timestamp_scenario;not null"`
	InputTokens  int       `gorm:"column:input_tokens;not null"`
	OutputTokens int       `gorm:"column:output_tokens;not null"`
	TotalTokens  int       `gorm:"column:total_tokens;index;not null"`
	Status       string    `gorm:"column:status;index;not null"` // success, error, partial
	ErrorCode    string    `gorm:"column:error_code"`
	LatencyMs    int       `gorm:"column:latency_ms"`
	Streamed     bool      `gorm:"column:streamed;default:0"`
}

// TableName specifies the table name for GORM
func (UsageRecord) TableName() string {
	return "usage_records"
}

// UsageDailyRecord is the GORM model for daily aggregated usage statistics
type UsageDailyRecord struct {
	ID           uint      `gorm:"primaryKey;autoIncrement;column:id"`
	Date         time.Time `gorm:"column:date;index:idx_date;index:idx_date_provider_model;not null"`
	ProviderUUID string    `gorm:"column:provider_uuid;index:idx_date_provider_model;not null"`
	ProviderName string    `gorm:"column:provider_name;not null"`
	Model        string    `gorm:"column:model;index:idx_date_provider_model;not null"`
	RequestCount int64     `gorm:"column:request_count;not null"`
	TotalTokens  int64     `gorm:"column:total_tokens;not null"`
	InputTokens  int64     `gorm:"column:input_tokens;not null"`
	OutputTokens int64     `gorm:"column:output_tokens;not null"`
	ErrorCount   int64     `gorm:"column:error_count;default:0"`
}

// TableName specifies the table name for GORM
func (UsageDailyRecord) TableName() string {
	return "usage_daily"
}

// UsageMonthlyRecord is the GORM model for monthly aggregated usage statistics
type UsageMonthlyRecord struct {
	ID           uint   `gorm:"primaryKey;autoIncrement;column:id"`
	Year         int    `gorm:"column:year;not null"`
	Month        int    `gorm:"column:month;not null"`
	ProviderUUID string `gorm:"column:provider_uuid;not null"`
	ProviderName string `gorm:"column:provider_name;not null"`
	Model        string `gorm:"column:model;not null"`
	RequestCount int64  `gorm:"column:request_count;not null"`
	TotalTokens  int64  `gorm:"column:total_tokens;not null"`
	InputTokens  int64  `gorm:"column:input_tokens;not null"`
	OutputTokens int64  `gorm:"column:output_tokens;not null"`
	ErrorCount   int64  `gorm:"column:error_count;default:0"`
}

// TableName specifies the table name for GORM
func (UsageMonthlyRecord) TableName() string {
	return "usage_monthly"
}

// UsageStore persists usage records in SQLite using GORM.
type UsageStore struct {
	db     *gorm.DB
	dbPath string
	mu     sync.Mutex
}

// NewUsageStore creates or loads a usage store using SQLite database.
func NewUsageStore(baseDir string) (*UsageStore, error) {
	log.Printf("Initializing usage store in directory: %s", baseDir)
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create usage store directory: %w", err)
	}

	dbPath := constant.GetDBFile(baseDir)
	log.Printf("Opening SQLite database for usage store: %s", dbPath)
	dsn := dbPath + "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=1"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open usage database: %w", err)
	}
	log.Printf("SQLite database opened successfully for usage store")

	store := &UsageStore{
		db:     db,
		dbPath: dbPath,
	}

	// Auto-migrate schema for all usage-related tables
	if err := db.AutoMigrate(&UsageRecord{}, &UsageDailyRecord{}, &UsageMonthlyRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate usage database: %w", err)
	}
	log.Printf("Usage store initialization completed")

	return store, nil
}

// RecordUsage records a single usage event
func (us *UsageStore) RecordUsage(record *UsageRecord) error {
	if record == nil {
		return errors.New("record cannot be nil")
	}

	us.mu.Lock()
	defer us.mu.Unlock()

	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now()
	}
	record.TotalTokens = record.InputTokens + record.OutputTokens
	if record.Status == "" {
		record.Status = "success"
	}

	return us.db.Create(record).Error
}

// GetAggregatedStats returns aggregated usage statistics based on query parameters
type UsageStatsQuery struct {
	GroupBy   string // model, provider, scenario, rule, daily, hourly
	StartTime time.Time
	EndTime   time.Time
	Provider  string
	Model     string
	Scenario  string
	RuleUUID  string
	Status    string
	Limit     int
	SortBy    string // total_tokens, request_count, avg_latency
	SortOrder string // asc, desc
}

// AggregatedStat represents aggregated usage statistics
type AggregatedStat struct {
	Key             string  `json:"key"`
	ProviderUUID    string  `json:"provider_uuid,omitempty"`
	ProviderName    string  `json:"provider_name,omitempty"`
	Model           string  `json:"model,omitempty"`
	Scenario        string  `json:"scenario,omitempty"`
	RequestCount    int64   `json:"request_count"`
	TotalTokens     int64   `json:"total_tokens"`
	InputTokens     int64   `json:"total_input_tokens"`
	OutputTokens    int64   `json:"total_output_tokens"`
	AvgInputTokens  float64 `json:"avg_input_tokens"`
	AvgOutputTokens float64 `json:"avg_output_tokens"`
	AvgLatencyMs    float64 `json:"avg_latency_ms"`
	ErrorCount      int64   `json:"error_count"`
	ErrorRate       float64 `json:"error_rate"`
	StreamedCount   int64   `json:"streamed_count"`
	StreamedRate    float64 `json:"streamed_rate"`
}

// GetAggregatedStats returns aggregated statistics
func (us *UsageStore) GetAggregatedStats(query UsageStatsQuery) ([]AggregatedStat, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	// Build the base query
	db := us.db.Model(&UsageRecord{})

	// Apply time filter
	if !query.StartTime.IsZero() {
		db = db.Where("timestamp >= ?", query.StartTime)
	}
	if !query.EndTime.IsZero() {
		db = db.Where("timestamp <= ?", query.EndTime)
	}

	// Apply filters
	if query.Provider != "" {
		db = db.Where("provider_uuid = ?", query.Provider)
	}
	if query.Model != "" {
		db = db.Where("model = ?", query.Model)
	}
	if query.Scenario != "" {
		db = db.Where("scenario = ?", query.Scenario)
	}
	if query.RuleUUID != "" {
		db = db.Where("rule_uuid = ?", query.RuleUUID)
	}
	if query.Status != "" {
		db = db.Where("status = ?", query.Status)
	}

	// Determine grouping and select fields
	var groupBy string
	var keyField string
	switch query.GroupBy {
	case "provider":
		groupBy = "provider_uuid, provider_name"
		keyField = "provider_uuid"
	case "scenario":
		groupBy = "scenario"
		keyField = "scenario"
	case "rule":
		groupBy = "rule_uuid"
		keyField = "rule_uuid"
	case "daily":
		groupBy = "date(timestamp)"
		keyField = "date(timestamp)"
	case "hourly":
		groupBy = "strftime('%Y-%m-%d %H:00:00', timestamp)"
		keyField = "strftime('%Y-%m-%d %H:00:00', timestamp)"
	default: // model
		groupBy = "provider_uuid, provider_name, model"
		keyField = "model"
	}

	type result struct {
		Key           string
		ProviderUUID  string
		ProviderName  string
		Model         string
		Scenario      string
		RequestCount  int64
		TotalTokens   int64
		InputTokens   int64
		OutputTokens  int64
		ErrorCount    int64
		StreamedCount int64
		AvgLatency    float64
	}

	var results []result
	selectClause := fmt.Sprintf(`
		%s as key,
		COALESCE(provider_uuid, '') as provider_uuid,
		COALESCE(provider_name, '') as provider_name,
		COALESCE(model, '') as model,
		COALESCE(scenario, '') as scenario,
		COUNT(*) as request_count,
		COALESCE(SUM(total_tokens), 0) as total_tokens,
		COALESCE(SUM(input_tokens), 0) as input_tokens,
		COALESCE(SUM(output_tokens), 0) as output_tokens,
		COALESCE(SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END), 0) as error_count,
		COALESCE(SUM(CASE WHEN streamed = 1 THEN 1 ELSE 0 END), 0) as streamed_count,
		COALESCE(AVG(latency_ms), 0) as avg_latency
	`, keyField)

	if err := db.
		Select(selectClause).
		Group(groupBy).
		Order(buildOrderBy(query.SortBy, query.SortOrder)).
		Limit(query.Limit).
		Scan(&results).Error; err != nil {
		return nil, err
	}

	// Convert to AggregatedStat
	stats := make([]AggregatedStat, len(results))
	for i, r := range results {
		stats[i] = AggregatedStat{
			Key:             r.Key,
			ProviderUUID:    r.ProviderUUID,
			ProviderName:    r.ProviderName,
			Model:           r.Model,
			Scenario:        r.Scenario,
			RequestCount:    r.RequestCount,
			TotalTokens:     r.TotalTokens,
			InputTokens:     r.InputTokens,
			OutputTokens:    r.OutputTokens,
			AvgInputTokens:  avgFloat(float64(r.InputTokens), r.RequestCount),
			AvgOutputTokens: avgFloat(float64(r.OutputTokens), r.RequestCount),
			AvgLatencyMs:    r.AvgLatency,
			ErrorCount:      r.ErrorCount,
			ErrorRate:       rateFloat(r.ErrorCount, r.RequestCount),
			StreamedCount:   r.StreamedCount,
			StreamedRate:    rateFloat(r.StreamedCount, r.RequestCount),
		}
	}

	return stats, nil
}

// TimeSeriesData represents a single time bucket in time series data
type TimeSeriesData struct {
	Timestamp    string  `json:"timestamp"`
	RequestCount int64   `json:"request_count"`
	TotalTokens  int64   `json:"total_tokens"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	ErrorCount   int64   `json:"error_count"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
}

// GetTimeSeries returns time-series data for usage
func (us *UsageStore) GetTimeSeries(interval string, startTime, endTime time.Time, filters map[string]string) ([]TimeSeriesData, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	var timeFormat string
	switch interval {
	case "minute":
		timeFormat = "%Y-%m-%d %H:%M:00"
	case "hour":
		timeFormat = "%Y-%m-%d %H:00:00"
	case "day":
		timeFormat = "%Y-%m-%d"
	case "week":
		timeFormat = "%Y-%W"
	default:
		timeFormat = "%Y-%m-%d %H:00:00" // default to hour
	}

	// Build query
	db := us.db.Model(&UsageRecord{})

	if !startTime.IsZero() {
		db = db.Where("timestamp >= ?", startTime)
	}
	if !endTime.IsZero() {
		db = db.Where("timestamp <= ?", endTime)
	}

	for key, value := range filters {
		db = db.Where(key+" = ?", value)
	}

	type result struct {
		Timestamp    string
		RequestCount int64
		TotalTokens  int64
		InputTokens  int64
		OutputTokens int64
		ErrorCount   int64
		AvgLatency   float64
	}

	var results []result
	selectClause := fmt.Sprintf(`
		strftime('%s', timestamp) as timestamp,
		COUNT(*) as request_count,
		COALESCE(SUM(total_tokens), 0) as total_tokens,
		COALESCE(SUM(input_tokens), 0) as input_tokens,
		COALESCE(SUM(output_tokens), 0) as output_tokens,
		COALESCE(SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END), 0) as error_count,
		COALESCE(AVG(latency_ms), 0) as avg_latency
	`, timeFormat)

	if err := db.
		Select(selectClause).
		Group("timestamp").
		Order("timestamp ASC").
		Scan(&results).Error; err != nil {
		return nil, err
	}

	// Convert to TimeSeriesData
	data := make([]TimeSeriesData, len(results))
	for i, r := range results {
		data[i] = TimeSeriesData{
			Timestamp:    r.Timestamp,
			RequestCount: r.RequestCount,
			TotalTokens:  r.TotalTokens,
			InputTokens:  r.InputTokens,
			OutputTokens: r.OutputTokens,
			ErrorCount:   r.ErrorCount,
			AvgLatencyMs: r.AvgLatency,
		}
	}

	return data, nil
}

// GetRecords returns individual usage records (for debugging/audit)
func (us *UsageStore) GetRecords(startTime, endTime time.Time, filters map[string]string, limit, offset int) ([]UsageRecord, int64, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	db := us.db.Model(&UsageRecord{})

	if !startTime.IsZero() {
		db = db.Where("timestamp >= ?", startTime)
	}
	if !endTime.IsZero() {
		db = db.Where("timestamp <= ?", endTime)
	}

	for key, value := range filters {
		db = db.Where(key+" = ?", value)
	}

	// Get total count
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get records with pagination
	var records []UsageRecord
	if err := db.
		Order("timestamp DESC").
		Limit(limit).
		Offset(offset).
		Find(&records).Error; err != nil {
		return nil, 0, err
	}

	return records, total, nil
}

// DeleteOlderThan deletes records older than the specified date
func (us *UsageStore) DeleteOlderThan(cutoffDate time.Time) (int64, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	result := us.db.Where("timestamp < ?", cutoffDate).Delete(&UsageRecord{})
	return result.RowsAffected, result.Error
}

// AggregateToDaily aggregates records to daily summaries
func (us *UsageStore) AggregateToDaily(date time.Time) (int64, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	// Aggregate usage records to daily summaries
	result := us.db.Exec(`
		INSERT OR REPLACE INTO usage_daily (date, provider_uuid, provider_name, model, request_count, total_tokens, input_tokens, output_tokens, error_count)
		SELECT
			date(?) as date,
			provider_uuid,
			provider_name,
			model,
			COUNT(*) as request_count,
			SUM(total_tokens) as total_tokens,
			SUM(input_tokens) as input_tokens,
			SUM(output_tokens) as output_tokens,
			SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) as error_count
		FROM usage_records
		WHERE date(timestamp) = date(?)
		GROUP BY provider_uuid, provider_name, model
	`, date, date)

	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// Helper functions
func buildOrderBy(sortBy, sortOrder string) string {
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	switch sortBy {
	case "request_count":
		return fmt.Sprintf("request_count %s", sortOrder)
	case "avg_latency":
		return fmt.Sprintf("avg_latency %s", sortOrder)
	default: // total_tokens
		return fmt.Sprintf("total_tokens %s", sortOrder)
	}
}

func avgFloat(sum float64, count int64) float64 {
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

func rateFloat(numerator, denominator int64) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
