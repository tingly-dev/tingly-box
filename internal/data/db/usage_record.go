package db

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/tingly-dev/tingly-box/internal/constant"
)

// UsageRecord is the GORM model for persisting individual usage records
type UsageRecord struct {
	ID           uint      `gorm:"primaryKey;autoIncrement;column:id"`
	ProviderUUID string    `gorm:"column:provider_uuid;index:idx_provider_model;not null"`
	ProviderName string    `gorm:"column:provider_name;not null"`
	Model        string    `gorm:"column:model;index:idx_provider_model;not null"`
	Scenario     string    `gorm:"column:scenario;index:idx_scenario;not null"`
	RuleUUID     string    `gorm:"column:rule_uuid;index:idx_rule"`
	UserID       string    `gorm:"column:user_id;index:idx_user;not null;default:''"`
	// TaskID / RunID attribute gateway traffic to a task-board run. They are
	// populated from the X-Tingly-Task-Id / X-Tingly-Run-Id request headers
	// that task executors inject into their agent CLI environment.
	TaskID       string    `gorm:"column:task_id;index:idx_task"`
	RunID        string    `gorm:"column:run_id"`
	RequestModel string    `gorm:"column:request_model"`
	Timestamp    time.Time `gorm:"column:timestamp;index:idx_timestamp;index:idx_timestamp_scenario;not null"`
	InputTokens  int       `gorm:"column:input_tokens;not null"`
	OutputTokens int       `gorm:"column:output_tokens;not null"`
	TotalTokens  int       `gorm:"column:total_tokens;index;not null"`
	// Cache tokens (combined cache creation and read)
	CacheInputTokens int `gorm:"column:cache_input_tokens;default:0"`
	// System tokens (framework overhead, templates, etc.)
	SystemTokens int    `gorm:"column:system_tokens;default:0"`
	Status       string `gorm:"column:status;index;not null"` // success, error, partial
	ErrorCode    string `gorm:"column:error_code"`
	LatencyMs    int    `gorm:"column:latency_ms"`
	TTFTMs       int    `gorm:"column:ttft_ms;default:0"`
	Streamed     bool   `gorm:"column:streamed;type:integer"`
}

const (
	// DefaultAdminUserID is the user ID for the default admin user
	// This is used for usage records created before multi-tenant support
	DefaultAdminUserID = "admin"
)

// TableName specifies the table name for GORM
func (UsageRecord) TableName() string {
	return "usage_records"
}

// UsageDailyRecord is the GORM model for daily aggregated usage statistics.
// One row per (UTC day, provider, model, user). Date uses the same day
// boundary as SQLite's date(timestamp) so daily rows can substitute raw
// usage_records scans for completed days.
type UsageDailyRecord struct {
	ID           uint   `gorm:"primaryKey;autoIncrement;column:id"`
	Date         string `gorm:"column:date;index:idx_date;uniqueIndex:uq_daily_dim,priority:1;not null"` // YYYY-MM-DD (UTC)
	ProviderUUID string `gorm:"column:provider_uuid;uniqueIndex:uq_daily_dim,priority:2;not null"`
	ProviderName string `gorm:"column:provider_name;not null"`
	Model        string `gorm:"column:model;uniqueIndex:uq_daily_dim,priority:3;not null"`
	UserID       string `gorm:"column:user_id;uniqueIndex:uq_daily_dim,priority:4;not null;default:''"`
	RequestCount int64  `gorm:"column:request_count;not null"`
	TotalTokens  int64  `gorm:"column:total_tokens;not null"`
	InputTokens  int64  `gorm:"column:input_tokens;not null"`
	OutputTokens int64  `gorm:"column:output_tokens;not null"`
	// Cache tokens
	CacheInputTokens int64 `gorm:"column:cache_input_tokens;default:0"`
	// System tokens
	SystemTokens  int64 `gorm:"column:system_tokens;default:0"`
	ErrorCount    int64 `gorm:"column:error_count;default:0"`
	StreamedCount int64 `gorm:"column:streamed_count;default:0"`
	// Sum of latency_ms across the day, so merged averages stay weighted
	LatencySumMs int64 `gorm:"column:latency_sum_ms;default:0"`
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
	// Cache tokens
	CacheInputTokens int64 `gorm:"column:cache_input_tokens;default:0"`
	// System tokens
	SystemTokens int64 `gorm:"column:system_tokens;default:0"`
	ErrorCount   int64 `gorm:"column:error_count;default:0"`
}

// TableName specifies the table name for GORM
func (UsageMonthlyRecord) TableName() string {
	return "usage_monthly"
}

// UsageStore persists usage records in SQLite using GORM.
type UsageStore struct {
	db     *gorm.DB
	dbPath string
	// mu guards the database: writes take the write lock, queries the read
	// lock (WAL mode supports concurrent readers), so dashboard queries do
	// not serialize behind proxy usage writes or each other.
	mu sync.RWMutex
}

// NewUsageStore creates or loads a usage store using SQLite database.
func NewUsageStore(baseDir string) (*UsageStore, error) {
	logrus.Printf("Initializing usage store in directory: %s", baseDir)
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create usage store directory: %w", err)
	}

	dbPath := constant.GetDBFile(baseDir)
	logrus.Printf("Opening SQLite database for usage store: %s", dbPath)
	dsn := dbPath + "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=1"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open usage database: %w", err)
	}
	logrus.Debugf("SQLite database opened successfully for usage store")

	store := &UsageStore{
		db:     db,
		dbPath: dbPath,
	}

	if err := migrateUsageTables(db); err != nil {
		return nil, err
	}
	if err := ensureUsageRecordSchema(db); err != nil {
		return nil, fmt.Errorf("failed to align usage schema: %w", err)
	}
	logrus.Debugf("Usage store initialization completed")

	return store, nil
}

// migrateUsageTables aligns and auto-migrates usage_records, usage_daily, and
// usage_monthly. Shared by NewUsageStore and StoreManager.initUsageStore so
// the two initialization paths can't drift apart.
func migrateUsageTables(db *gorm.DB) error {
	if err := ensureUsageDailySchema(db); err != nil {
		return fmt.Errorf("failed to align usage daily schema: %w", err)
	}
	if err := db.AutoMigrate(&UsageRecord{}, &UsageDailyRecord{}, &UsageMonthlyRecord{}); err != nil {
		return fmt.Errorf("failed to migrate usage database: %w", err)
	}
	return nil
}

func ensureUsageRecordSchema(db *gorm.DB) error {
	// Dev-stage breaking cleanup: remove deprecated department_id dimension.
	if db.Migrator().HasColumn(&UsageRecord{}, "department_id") {
		if err := db.Migrator().DropColumn(&UsageRecord{}, "department_id"); err != nil {
			return err
		}
	}
	// Migrate from separate cache fields to combined cache_input_tokens
	if db.Migrator().HasColumn(&UsageRecord{}, "cache_creation_input_tokens") {
		// Add new column if it doesn't exist
		if !db.Migrator().HasColumn(&UsageRecord{}, "cache_input_tokens") {
			if err := db.Migrator().AutoMigrate(&UsageRecord{}); err != nil {
				return err
			}
		}
		// Migrate data: sum of cache_creation + cache_read
		if err := db.Exec(`
			UPDATE usage_records
			SET cache_input_tokens = COALESCE(cache_creation_input_tokens, 0) + COALESCE(cache_read_input_tokens, 0)
			WHERE cache_input_tokens = 0
		`).Error; err != nil {
			return err
		}
		// Drop old columns
		if err := db.Migrator().DropColumn(&UsageRecord{}, "cache_creation_input_tokens"); err != nil {
			return err
		}
		if err := db.Migrator().DropColumn(&UsageRecord{}, "cache_read_input_tokens"); err != nil {
			return err
		}
	}

	// Migrate empty user_id to default admin user
	// This ensures backward compatibility after multi-tenant support was added
	// Records created before multi-tenant have empty user_id, which should be
	// associated with the default admin user
	if err := db.Exec(`
		UPDATE usage_records
		SET user_id = ?
		WHERE user_id = '' OR user_id IS NULL
	`, DefaultAdminUserID).Error; err != nil {
		logrus.WithError(err).Warn("Failed to migrate empty user_id to default admin user")
		// Don't fail initialization for this migration, it's not critical
	}

	return nil
}

// ensureUsageDailySchema rebuilds the usage_daily table when it predates the
// v2 layout (user_id dimension + streamed/latency sums). The table holds only
// derived data and is repopulated lazily, so dropping it is safe.
func ensureUsageDailySchema(db *gorm.DB) error {
	m := db.Migrator()
	if m.HasTable(&UsageDailyRecord{}) && !m.HasColumn(&UsageDailyRecord{}, "user_id") {
		if err := m.DropTable(&UsageDailyRecord{}); err != nil {
			return err
		}
	}
	return nil
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
	// Total tokens is the sum of all token types
	record.TotalTokens = record.InputTokens + record.OutputTokens
	if record.Status == "" {
		record.Status = "success"
	}

	return us.db.Create(record).Error
}

// RenameRuleUUID re-attributes historical usage records from oldUUID to
// newUUID so per-rule usage stats survive a rule UUID normalization.
func (us *UsageStore) RenameRuleUUID(oldUUID, newUUID string) error {
	us.mu.Lock()
	defer us.mu.Unlock()

	return us.db.Model(&UsageRecord{}).
		Where("rule_uuid = ?", oldUUID).
		Update("rule_uuid", newUUID).Error
}

// GetAggregatedStats returns aggregated usage statistics based on query parameters
type UsageStatsQuery struct {
	GroupBy   string // model, provider, scenario, rule, user, daily, hourly
	StartTime time.Time
	EndTime   time.Time
	Provider  string
	Model     string
	Scenario  string
	RuleUUID  string
	UserID    string
	Status    string
	Limit     int
	SortBy    string // total_tokens, request_count, avg_latency
	SortOrder string // asc, desc
}

// AggregatedStat represents aggregated usage statistics
type AggregatedStat struct {
	Key              string  `json:"key"`
	ProviderUUID     string  `json:"provider_uuid,omitempty"`
	ProviderName     string  `json:"provider_name,omitempty"`
	Model            string  `json:"model,omitempty"`
	Scenario         string  `json:"scenario,omitempty"`
	UserID           string  `json:"user_id,omitempty"`
	RequestCount     int64   `json:"request_count"`
	TotalTokens      int64   `json:"total_tokens"`
	InputTokens      int64   `json:"total_input_tokens"`
	OutputTokens     int64   `json:"total_output_tokens"`
	CacheInputTokens int64   `json:"cache_input_tokens"`
	SystemTokens     int64   `json:"system_tokens"`
	AvgInputTokens   float64 `json:"avg_input_tokens"`
	AvgOutputTokens  float64 `json:"avg_output_tokens"`
	AvgLatencyMs     float64 `json:"avg_latency_ms"`
	ErrorCount       int64   `json:"error_count"`
	ErrorRate        float64 `json:"error_rate"`
	StreamedCount    int64   `json:"streamed_count"`
	StreamedRate     float64 `json:"streamed_rate"`
}

// GetAggregatedStats returns aggregated statistics. For queries spanning
// several completed days it combines the usage_daily pre-aggregation table
// with a raw scan of only the partial edge days (see usage_daily.go), so
// dashboard loads stay fast regardless of how many raw records accumulate.
func (us *UsageStore) GetAggregatedStats(query UsageStatsQuery) ([]AggregatedStat, error) {
	if stats, ok, err := us.aggregatedStatsFromDaily(query); ok {
		return stats, err
	}

	buckets, err := us.rawAggBuckets(query, true)
	if err != nil {
		return nil, err
	}
	stats := make([]AggregatedStat, len(buckets))
	for i, b := range buckets {
		stats[i] = b.toAggregatedStat()
	}
	return stats, nil
}

// aggBucket carries additive aggregation sums so results from usage_daily and
// raw usage_records scans can be merged before computing derived rates.
type aggBucket struct {
	Key              string
	ProviderUUID     string
	ProviderName     string
	Model            string
	Scenario         string
	UserID           string
	RequestCount     int64
	TotalTokens      int64
	InputTokens      int64
	OutputTokens     int64
	CacheInputTokens int64
	SystemTokens     int64
	ErrorCount       int64
	StreamedCount    int64
	LatencySum       int64
}

func (b aggBucket) toAggregatedStat() AggregatedStat {
	return AggregatedStat{
		Key:              b.Key,
		ProviderUUID:     b.ProviderUUID,
		ProviderName:     b.ProviderName,
		Model:            b.Model,
		Scenario:         b.Scenario,
		UserID:           b.UserID,
		RequestCount:     b.RequestCount,
		TotalTokens:      b.TotalTokens,
		InputTokens:      b.InputTokens,
		OutputTokens:     b.OutputTokens,
		CacheInputTokens: b.CacheInputTokens,
		SystemTokens:     b.SystemTokens,
		AvgInputTokens:   avgFloat(float64(b.InputTokens), b.RequestCount),
		AvgOutputTokens:  avgFloat(float64(b.OutputTokens), b.RequestCount),
		AvgLatencyMs:     avgFloat(float64(b.LatencySum), b.RequestCount),
		ErrorCount:       b.ErrorCount,
		ErrorRate:        rateFloat(b.ErrorCount, b.RequestCount),
		StreamedCount:    b.StreamedCount,
		StreamedRate:     rateFloat(b.StreamedCount, b.RequestCount),
	}
}

// rawAggBuckets aggregates directly over usage_records. When applyLimit is
// false, sorting/limiting is left to the caller (used for edge-day scans that
// are merged with usage_daily results afterwards).
func (us *UsageStore) rawAggBuckets(query UsageStatsQuery, applyLimit bool) ([]aggBucket, error) {
	us.mu.RLock()
	defer us.mu.RUnlock()

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
	if query.UserID != "" {
		db = db.Where("user_id = ?", query.UserID)
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
	case "user":
		groupBy = "user_id"
		keyField = "user_id"
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

	var results []aggBucket
	selectClause := fmt.Sprintf(`
		%s as key,
		COALESCE(provider_uuid, '') as provider_uuid,
		COALESCE(provider_name, '') as provider_name,
		COALESCE(model, '') as model,
		COALESCE(scenario, '') as scenario,
		COALESCE(user_id, '') as user_id,
		COUNT(*) as request_count,
		COALESCE(SUM(total_tokens), 0) as total_tokens,
		COALESCE(SUM(input_tokens), 0) as input_tokens,
		COALESCE(SUM(output_tokens), 0) as output_tokens,
		COALESCE(SUM(cache_input_tokens), 0) as cache_input_tokens,
		COALESCE(SUM(system_tokens), 0) as system_tokens,
		COALESCE(SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END), 0) as error_count,
		COALESCE(SUM(CASE WHEN streamed = true THEN 1 ELSE 0 END), 0) as streamed_count,
		COALESCE(SUM(latency_ms), 0) as latency_sum
	`, keyField)

	db = db.Select(selectClause).Group(groupBy)
	if applyLimit {
		db = db.Order(buildOrderBy(query.SortBy, query.SortOrder)).Limit(query.Limit)
	}
	if err := db.Scan(&results).Error; err != nil {
		return nil, err
	}

	return results, nil
}

// TimeSeriesData represents a single time bucket in time series data
type TimeSeriesData struct {
	Timestamp        string  `json:"timestamp"`
	RequestCount     int64   `json:"request_count"`
	TotalTokens      int64   `json:"total_tokens"`
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	CacheInputTokens int64   `json:"cache_input_tokens"`
	SystemTokens     int64   `json:"system_tokens"`
	ErrorCount       int64   `json:"error_count"`
	AvgLatencyMs     float64 `json:"avg_latency_ms"`
}

// GetTimeSeries returns time-series data for usage. Day-interval queries
// spanning several completed days are served from usage_daily with raw scans
// only for the partial edge days (see usage_daily.go).
func (us *UsageStore) GetTimeSeries(interval string, startTime, endTime time.Time, filters map[string]string) ([]TimeSeriesData, error) {
	if data, ok, err := us.timeSeriesFromDaily(interval, startTime, endTime, filters); ok {
		return data, err
	}
	return us.rawTimeSeries(interval, startTime, endTime, filters)
}

// rawTimeSeries aggregates time buckets directly over usage_records.
func (us *UsageStore) rawTimeSeries(interval string, startTime, endTime time.Time, filters map[string]string) ([]TimeSeriesData, error) {
	us.mu.RLock()
	defer us.mu.RUnlock()

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
		Timestamp        string
		RequestCount     int64
		TotalTokens      int64
		InputTokens      int64
		OutputTokens     int64
		CacheInputTokens int64
		SystemTokens     int64
		ErrorCount       int64
		AvgLatency       float64
	}

	var results []result
	// Select the Unix timestamp of the time bucket (the grouped time), not the original timestamp
	selectClause := fmt.Sprintf(`
		strftime('%%s', strftime('%s', timestamp)) as timestamp,
		COUNT(*) as request_count,
		COALESCE(SUM(total_tokens), 0) as total_tokens,
		COALESCE(SUM(input_tokens), 0) as input_tokens,
		COALESCE(SUM(output_tokens), 0) as output_tokens,
		COALESCE(SUM(cache_input_tokens), 0) as cache_input_tokens,
		COALESCE(SUM(system_tokens), 0) as system_tokens,
		COALESCE(SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END), 0) as error_count,
		COALESCE(AVG(latency_ms), 0) as avg_latency
	`, timeFormat)

	if err := db.
		Select(selectClause).
		Group(fmt.Sprintf("strftime('%s', timestamp)", timeFormat)).
		Order("timestamp ASC").
		Scan(&results).Error; err != nil {
		return nil, err
	}

	// Convert to TimeSeriesData
	data := make([]TimeSeriesData, len(results))
	for i, r := range results {
		data[i] = TimeSeriesData{
			Timestamp:        r.Timestamp,
			RequestCount:     r.RequestCount,
			TotalTokens:      r.TotalTokens,
			InputTokens:      r.InputTokens,
			OutputTokens:     r.OutputTokens,
			CacheInputTokens: r.CacheInputTokens,
			SystemTokens:     r.SystemTokens,
			ErrorCount:       r.ErrorCount,
			AvgLatencyMs:     r.AvgLatency,
		}
	}

	return data, nil
}

// GetRecords returns individual usage records (for debugging/audit)
func (us *UsageStore) GetRecords(startTime, endTime time.Time, filters map[string]string, limit, offset int) ([]UsageRecord, int64, error) {
	us.mu.RLock()
	defer us.mu.RUnlock()

	base := func() *gorm.DB {
		q := us.db.Model(&UsageRecord{})
		if !startTime.IsZero() {
			q = q.Where("timestamp >= ?", startTime)
		}
		if !endTime.IsZero() {
			q = q.Where("timestamp <= ?", endTime)
		}
		for key, value := range filters {
			q = q.Where(key+" = ?", value)
		}
		return q
	}

	// Get records with pagination
	var records []UsageRecord
	if err := base().
		Order("timestamp DESC").
		Limit(limit).
		Offset(offset).
		Find(&records).Error; err != nil {
		return nil, 0, err
	}

	// The full COUNT(*) scan is only needed when the page might not contain
	// everything; the common dashboard case (first page, under the limit)
	// gets the total for free.
	total := int64(offset + len(records))
	if len(records) == limit {
		if err := base().Count(&total).Error; err != nil {
			return nil, 0, err
		}
	}

	return records, total, nil
}

// GetRecordsAfterID returns usage records with id greater than lastID.
// On initial sync, startTime can be used to cap the historical backfill window.
func (us *UsageStore) GetRecordsAfterID(lastID uint, startTime time.Time, limit int) ([]UsageRecord, error) {
	us.mu.RLock()
	defer us.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	db := us.db.Model(&UsageRecord{}).Where("id > ?", lastID)
	if !startTime.IsZero() {
		db = db.Where("timestamp >= ?", startTime)
	}

	var records []UsageRecord
	if err := db.
		Order("id ASC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, err
	}

	return records, nil
}

// DeleteOlderThan deletes records older than the specified date, together
// with the daily aggregates derived from them so both views stay consistent.
func (us *UsageStore) DeleteOlderThan(cutoffDate time.Time) (int64, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	result := us.db.Where("timestamp < ?", cutoffDate).Delete(&UsageRecord{})
	if result.Error == nil {
		// Include the boundary day: its aggregate now over-counts deleted
		// records and will be rebuilt from the remaining raw rows on the
		// next query.
		us.db.Where("date <= ?", cutoffDate.UTC().Format(dailyDateLayout)).Delete(&UsageDailyRecord{})
	}
	return result.RowsAffected, result.Error
}

// AggregateToDaily (re)builds the usage_daily rows for the UTC day containing
// the given time. It returns the number of aggregate rows written.
func (us *UsageStore) AggregateToDaily(date time.Time) (int64, error) {
	day := utcDayStart(date)
	return us.aggregateDay(day.Format(dailyDateLayout), day)
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
		return fmt.Sprintf("(latency_sum * 1.0 / request_count) %s", sortOrder)
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

// TaskUsageTotals aggregates gateway usage attributed to one task.
type TaskUsageTotals struct {
	TaskID           string `json:"task_id"`
	Requests         int64  `json:"requests"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheInputTokens int64  `json:"cache_input_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
}

// GetTaskUsageTotals sums usage records attributed to taskID.
func (us *UsageStore) GetTaskUsageTotals(taskID string) (*TaskUsageTotals, error) {
	us.mu.RLock()
	defer us.mu.RUnlock()

	totals := &TaskUsageTotals{TaskID: taskID}
	err := us.db.Model(&UsageRecord{}).
		Select("COUNT(*) AS requests, COALESCE(SUM(input_tokens),0) AS input_tokens, COALESCE(SUM(output_tokens),0) AS output_tokens, COALESCE(SUM(cache_input_tokens),0) AS cache_input_tokens, COALESCE(SUM(total_tokens),0) AS total_tokens").
		Where("task_id = ?", taskID).
		Scan(totals).Error
	if err != nil {
		return nil, err
	}
	return totals, nil
}
