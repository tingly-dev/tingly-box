package db

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"
)

// UsageRecord is the GORM model for persisting individual usage records.
//
// Indexes: every aggregate read is a timestamp range (idx_timestamp) narrowed
// by filters that withPinnedRangeFilters keeps off the index, so scenario,
// status and total_tokens carry none, and timestamp has no composite twin:
// each extra index is paid on every proxied request's INSERT (see
// droppedUsageIndexes for the ones removed from existing databases).
type UsageRecord struct {
	ID           uint      `gorm:"primaryKey;autoIncrement;column:id"`
	ProviderUUID string    `gorm:"column:provider_uuid;index:idx_provider_model;not null"`
	ProviderName string    `gorm:"column:provider_name;not null"`
	Model        string    `gorm:"column:model;index:idx_provider_model;not null"`
	Scenario     string    `gorm:"column:scenario;not null"`
	RuleUUID     string    `gorm:"column:rule_uuid;index:idx_rule"`
	UserID       string    `gorm:"column:user_id;index:idx_user;not null;default:''"`
	TeamID       string    `gorm:"column:team_id;index:idx_team;not null;default:''"`
	RequestModel string    `gorm:"column:request_model"`
	Timestamp    time.Time `gorm:"column:timestamp;index:idx_timestamp;not null"`
	InputTokens  int       `gorm:"column:input_tokens;not null"`
	OutputTokens int       `gorm:"column:output_tokens;not null"`
	TotalTokens  int       `gorm:"column:total_tokens;not null"`
	// CacheReadTokens counts cache-READ hits only. Cache writes are billed
	// separately (Anthropic cache_creation, OpenAI cache_write_tokens since
	// gpt-5.6) and are counted in CacheWriteTokens — which is a SUBSET of
	// InputTokens, not an addition to it.
	//
	// The physical column is still cache_input_tokens: renaming it would mean
	// an ALTER on three tables for no behavioral gain, so the legacy name is
	// pinned in the tag and the mismatch stops at this line. Raw SQL selects
	// alias it (`SUM(cache_input_tokens) as cache_read_tokens`) so Scan binds.
	CacheReadTokens  int `gorm:"column:cache_input_tokens;default:0"`
	CacheWriteTokens int `gorm:"column:cache_write_tokens;default:0"`
	// ReasoningTokens counts thinking/reasoning tokens (OpenAI reasoning
	// models, Anthropic extended thinking). A SUBSET of OutputTokens, not
	// an addition to it.
	ReasoningTokens int `gorm:"column:reasoning_tokens;default:0"`
	// System tokens (framework overhead, templates, etc.)
	SystemTokens int    `gorm:"column:system_tokens;default:0"`
	Status       string `gorm:"column:status;not null"` // success, error, partial
	ErrorCode    string `gorm:"column:error_code"`
	LatencyMs    int    `gorm:"column:latency_ms"`
	TTFTMs       int    `gorm:"column:ttft_ms;default:0"`
	Streamed     bool   `gorm:"column:streamed;type:integer"`
	// TraceID correlates this record with the OTel trace of the same request
	// so billing/audit rows can jump to the distributed trace and back.
	// Empty when tracing is disabled or the request span was not sampled.
	TraceID string `gorm:"column:trace_id;index:idx_trace_id"`
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

// RollupSums are the additive measures every usage rollup row carries, so
// results from different rollup tiers and from raw scans can be merged before
// derived rates (error rate, average latency) are computed.
type RollupSums struct {
	RequestCount int64 `gorm:"column:request_count;not null"`
	TotalTokens  int64 `gorm:"column:total_tokens;not null"`
	InputTokens  int64 `gorm:"column:input_tokens;not null"`
	OutputTokens int64 `gorm:"column:output_tokens;not null"`
	// Cache tokens: reads, then writes (a subset of InputTokens)
	CacheReadTokens  int64 `gorm:"column:cache_input_tokens;default:0"`
	CacheWriteTokens int64 `gorm:"column:cache_write_tokens;default:0"`
	// ReasoningTokens: a subset of OutputTokens (see UsageRecord doc)
	ReasoningTokens int64 `gorm:"column:reasoning_tokens;default:0"`
	// System tokens
	SystemTokens  int64 `gorm:"column:system_tokens;default:0"`
	ErrorCount    int64 `gorm:"column:error_count;default:0"`
	StreamedCount int64 `gorm:"column:streamed_count;default:0"`
	// Sum of latency_ms over the bucket, so merged averages stay weighted
	LatencySumMs int64 `gorm:"column:latency_sum_ms;default:0"`
}

// UsageHourlyRecord is the finest usage rollup: one row per (UTC hour,
// provider, model, user) that had traffic, plus a zero-count marker row for a
// completed hour with none (see rollup.go). Hour is SQLite's
// strftime('%Y-%m-%d %H', timestamp), so hour rows can substitute raw
// usage_records scans for completed hours. UsageDailyRecord is derived from
// these rows, never from raw records.
type UsageHourlyRecord struct {
	ID           uint   `gorm:"primaryKey;autoIncrement;column:id"`
	Hour         string `gorm:"column:hour;index:idx_hourly_hour;uniqueIndex:uq_hourly_dim,priority:1;not null"` // YYYY-MM-DD HH (UTC)
	ProviderUUID string `gorm:"column:provider_uuid;uniqueIndex:uq_hourly_dim,priority:2;not null"`
	ProviderName string `gorm:"column:provider_name;not null"`
	Model        string `gorm:"column:model;uniqueIndex:uq_hourly_dim,priority:3;not null"`
	UserID       string `gorm:"column:user_id;uniqueIndex:uq_hourly_dim,priority:4;not null;default:''"`
	RollupSums   `gorm:"embedded"`
}

// TableName specifies the table name for GORM
func (UsageHourlyRecord) TableName() string {
	return "usage_hourly"
}

// UsageHourlyScenarioRecord is the per-agent (scenario) companion of
// UsageHourlyRecord: one row per (UTC hour, scenario). See
// UsageDailyScenarioRecord for why scenario is not a dimension of the main
// table.
type UsageHourlyScenarioRecord struct {
	ID         uint   `gorm:"primaryKey;autoIncrement;column:id"`
	Hour       string `gorm:"column:hour;index:idx_hourly_scenario_hour;uniqueIndex:uq_hourly_scenario,priority:1;not null"` // YYYY-MM-DD HH (UTC)
	Scenario   string `gorm:"column:scenario;uniqueIndex:uq_hourly_scenario,priority:2;not null;default:''"`
	RollupSums `gorm:"embedded"`
}

// TableName specifies the table name for GORM
func (UsageHourlyScenarioRecord) TableName() string {
	return "usage_hourly_scenario"
}

// UsageDailyRecord is the GORM model for daily aggregated usage statistics.
// One row per (UTC day, provider, model, user) that had traffic, plus a
// zero-count marker row for a completed day with none (see rollup.go). Date
// uses the same day boundary as SQLite's date(timestamp) so daily rows can
// substitute raw usage_records scans for completed days. Rows are the sum of
// the day's UsageHourlyRecord rows.
type UsageDailyRecord struct {
	ID           uint   `gorm:"primaryKey;autoIncrement;column:id"`
	Date         string `gorm:"column:date;index:idx_date;uniqueIndex:uq_daily_dim,priority:1;not null"` // YYYY-MM-DD (UTC)
	ProviderUUID string `gorm:"column:provider_uuid;uniqueIndex:uq_daily_dim,priority:2;not null"`
	ProviderName string `gorm:"column:provider_name;not null"`
	Model        string `gorm:"column:model;uniqueIndex:uq_daily_dim,priority:3;not null"`
	UserID       string `gorm:"column:user_id;uniqueIndex:uq_daily_dim,priority:4;not null;default:''"`
	RollupSums   `gorm:"embedded"`
}

// TableName specifies the table name for GORM
func (UsageDailyRecord) TableName() string {
	return "usage_daily"
}

// UsageDailyScenarioRecord is the per-agent (scenario) companion of
// UsageDailyRecord: one row per (UTC day, scenario). It is a separate table,
// not another dimension on usage_daily, because crossing scenario with
// provider x model x user multiplied that table's rows several-fold and made
// every model/provider/user query slower to serve the one query (the agent
// filter options) that only needs scenario. It carries no provider/model/user,
// so it serves only queries that do not filter on them.
//
// Always written in the same transaction as the usage_daily rows of the same
// days, so a day is present in both or neither.
type UsageDailyScenarioRecord struct {
	ID         uint   `gorm:"primaryKey;autoIncrement;column:id"`
	Date       string `gorm:"column:date;index:idx_daily_scenario_date;uniqueIndex:uq_daily_scenario,priority:1;not null"` // YYYY-MM-DD (UTC)
	Scenario   string `gorm:"column:scenario;uniqueIndex:uq_daily_scenario,priority:2;not null;default:''"`
	RollupSums `gorm:"embedded"`
}

// TableName specifies the table name for GORM
func (UsageDailyScenarioRecord) TableName() string {
	return "usage_daily_scenario"
}

// UsageStore persists usage records in SQLite using GORM.
type UsageStore struct {
	storeConn
	// mu guards the database: writes take the write lock, queries the read
	// lock (WAL mode supports concurrent readers), so dashboard queries do
	// not serialize behind proxy usage writes or each other.
	mu sync.RWMutex

	// perf caches GetPerformanceSummary for perfCacheTTL (see
	// defaultPerfCacheTTL); anything that deletes or re-attributes records
	// resets it.
	perf         perfCache
	perfCacheTTL time.Duration
}

// PerformanceMetricSummary describes the typical and tail values for one
// request-performance metric. P10 is primarily useful for TPS, where lower is
// worse; latency-style metrics use P50/P90/P99.
type PerformanceMetricSummary struct {
	SampleCount int64
	P10         float64
	P50         float64
	P90         float64
	P95         float64
	P99         float64
}

// PerformanceSummary separates the three questions users ask about a streamed
// response: when it started, how quickly it generated, and when it completed.
type PerformanceSummary struct {
	TTFT       PerformanceMetricSummary
	TPS        PerformanceMetricSummary
	Completion PerformanceMetricSummary
}

// NewUsageStore creates or loads a usage store over its own connection to
// the shared tingly.db.
func NewUsageStore(baseDir string) (*UsageStore, error) {
	db, err := openTinglyDB(baseDir)
	if err != nil {
		return nil, fmt.Errorf("usage store: %w", err)
	}
	return newUsageStore(ownedConn(db))
}

// newUsageStore finishes setting up a UsageStore (migrate) over an
// already-open connection.
func newUsageStore(conn storeConn) (*UsageStore, error) {
	if err := migrateUsageTables(conn.db); err != nil {
		return nil, err
	}
	return &UsageStore{storeConn: conn, perfCacheTTL: defaultPerfCacheTTL}, nil
}

// migrateUsageTables aligns and auto-migrates usage_records, usage_daily and
// usage_daily_scenario.
// Shared by NewUsageStore and StoreManager.initUsageStore so the two
// initialization paths can't drift apart.
func migrateUsageTables(db *gorm.DB) error {
	if err := ensureUsageDailySchema(db); err != nil {
		return fmt.Errorf("failed to align usage daily schema: %w", err)
	}
	if err := dropUnusedUsageIndexes(db); err != nil {
		return fmt.Errorf("failed to drop unused usage indexes: %w", err)
	}
	if err := db.AutoMigrate(&UsageRecord{}, &UsageHourlyRecord{}, &UsageHourlyScenarioRecord{}, &UsageDailyRecord{}, &UsageDailyScenarioRecord{}); err != nil {
		return fmt.Errorf("failed to migrate usage database: %w", err)
	}
	return nil
}

// droppedUsageIndexes are usage_records indexes that earlier versions created
// and no query uses. AutoMigrate only ever adds indexes, so removing a tag
// from UsageRecord leaves the index on existing databases unless it is dropped
// here.
//
//   - idx_timestamp_scenario: a composite whose leading column is already
//     idx_timestamp's; the scenario half was never usable for the equality
//     filter that follows a range scan.
//   - idx_scenario, idx_usage_records_status: single low-cardinality columns
//     that the planner (no sqlite_stat1) wrongly preferred over the timestamp
//     range -- see withPinnedRangeFilters.
//   - idx_usage_records_total_tokens: nothing filters or sorts raw rows by it.
var droppedUsageIndexes = []string{
	"idx_timestamp_scenario",
	"idx_scenario",
	"idx_usage_records_status",
	"idx_usage_records_total_tokens",
}

func dropUnusedUsageIndexes(db *gorm.DB) error {
	m := db.Migrator()
	if !m.HasTable(&UsageRecord{}) {
		return nil
	}
	for _, name := range droppedUsageIndexes {
		if m.HasIndex(&UsageRecord{}, name) {
			if err := m.DropIndex(&UsageRecord{}, name); err != nil {
				return err
			}
		}
	}
	return nil
}

// ensureUsageDailySchema rebuilds the usage_daily table when it predates the
// v2 layout (user_id dimension + streamed/latency sums), or when it predates
// usage_daily_scenario (the two are filled together, so days aggregated before
// the companion table existed would never reach it). The table holds only
// derived data and is repopulated lazily, so dropping it is safe.
//
// A newly summed column does NOT belong in that condition. usage_daily and
// usage_records migrate together, so a column that is new here is new there
// too: every historical record contributes 0, which is exactly what
// AutoMigrate's zero-fill produces. Dropping would discard correct aggregates
// and force a full re-aggregation over the whole retention window for no gain.
// Only a column whose SOURCE already carries historical non-zero data — a
// split or backfill of an existing measure — needs the rebuild.
func ensureUsageDailySchema(db *gorm.DB) error {
	m := db.Migrator()
	if m.HasTable(&UsageDailyRecord{}) &&
		(!m.HasColumn(&UsageDailyRecord{}, "user_id") || !m.HasTable(&UsageDailyScenarioRecord{})) {
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

	prepareUsageRecord(record)

	return us.db.Create(record).Error
}

// prepareUsageRecord fills in RecordUsage's defaults without touching the
// database -- split out so RecordRequestOutcome can create it in the same
// transaction as a StatsStore write.
func prepareUsageRecord(record *UsageRecord) {
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now()
	}
	// Total tokens is the sum of all token types
	record.TotalTokens = record.InputTokens + record.OutputTokens
	if record.Status == "" {
		record.Status = "success"
	}
}

// RenameRuleUUID re-attributes historical usage records from oldUUID to
// newUUID so per-rule usage stats survive a rule UUID normalization.
func (us *UsageStore) RenameRuleUUID(oldUUID, newUUID string) error {
	us.mu.Lock()
	defer us.mu.Unlock()
	us.perf.reset()

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
	TeamID    string
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
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	ReasoningTokens  int64   `json:"reasoning_tokens"`
	SystemTokens     int64   `json:"system_tokens"`
	AvgInputTokens   float64 `json:"avg_input_tokens"`
	AvgOutputTokens  float64 `json:"avg_output_tokens"`
	AvgLatencyMs     float64 `json:"avg_latency_ms"`
	ErrorCount       int64   `json:"error_count"`
	ErrorRate        float64 `json:"error_rate"`
	StreamedCount    int64   `json:"streamed_count"`
	StreamedRate     float64 `json:"streamed_rate"`
}

// ---------- query scopes ----------
//
// Every read path over usage_records narrows by the same two things: a
// timestamp range and a set of equality filters. These scopes are that
// shared shape, so the four call sites (rawAggBuckets, rawTimeSeries,
// GetRecords, GetPerformanceSummary) cannot drift apart.

// withinTimeRange narrows to [start, end]. A zero bound is left open.
func withinTimeRange(start, end time.Time) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if !start.IsZero() {
			db = db.Where("timestamp >= ?", start)
		}
		if !end.IsZero() {
			db = db.Where("timestamp <= ?", end)
		}
		return db
	}
}

// usageFilterColumns is the closed set of columns the map-based filter API
// may narrow on.
//
// The filter map's *key* is a column name that gets interpolated into SQL.
// Every caller today builds that map with hardcoded keys and takes only the
// value from the request (see internal/server/module/usage/handler.go), so
// this is not a live injection, but nothing in the store enforced it and the
// next caller had no way to know. Validating here makes it structural.
//
// An unknown key is an error rather than a silently dropped filter: dropping
// one widens the result set, and for user_id that would mean handing back
// another user's records.
var usageFilterColumns = map[string]struct{}{
	"provider_uuid": {},
	"provider_name": {},
	"model":         {},
	"request_model": {},
	"scenario":      {},
	"rule_uuid":     {},
	"user_id":       {},
	"team_id":       {},
	"status":        {},
	"error_code":    {},
}

// validateFilterColumns rejects any filter key outside usageFilterColumns.
func validateFilterColumns(filters map[string]string) error {
	for key := range filters {
		if _, ok := usageFilterColumns[key]; !ok {
			return fmt.Errorf("usage: unsupported filter column %q", key)
		}
	}
	return nil
}

// withColumnFilters applies equality filters keyed by column name. Callers
// must have run validateFilterColumns first; keys are sorted so the generated
// SQL is stable (Go randomizes map iteration, which otherwise produces a
// different statement per call). The filters stay eligible for their own
// indexes: right for a paged, newest-first listing (GetRecords) where a rare
// filter should find its rows through idx_provider_model/idx_user/... rather
// than walking the time index.
func withColumnFilters(filters map[string]string) func(*gorm.DB) *gorm.DB {
	return columnFilters(filters, false)
}

// withPinnedRangeFilters is withColumnFilters for queries that must read
// every row of a timestamp range anyway (aggregates, time series, percentiles).
// The column is written "+col": SQLite's unary plus makes the term ineligible
// for an index, so the range index decides. Without sqlite_stat1 (nothing here
// runs ANALYZE) the planner treats any equality term as more selective than a
// range and walks e.g. the status index (95% of rows are "success") or the
// scenario index across ALL history, then filters by timestamp row by row -- a
// 30-day performance query read the whole table. Pinning the range index is
// deterministic where ANALYZE would be a guess that goes stale as the table
// grows.
func withPinnedRangeFilters(filters map[string]string) func(*gorm.DB) *gorm.DB {
	return columnFilters(filters, true)
}

func columnFilters(filters map[string]string, pinned bool) func(*gorm.DB) *gorm.DB {
	prefix := ""
	if pinned {
		prefix = "+"
	}
	return func(db *gorm.DB) *gorm.DB {
		for _, key := range slices.Sorted(maps.Keys(filters)) {
			db = db.Where(prefix+key+" = ?", filters[key])
		}
		return db
	}
}

// withStatsFilters is the UsageStatsQuery equivalent of
// withPinnedRangeFilters: same columns, taken from typed fields instead of a
// map, so no validation is needed.
func withStatsFilters(q UsageStatsQuery) func(*gorm.DB) *gorm.DB {
	return withPinnedRangeFilters(q.filterMap())
}

// filterMap renders the query's set dimension fields as a column filter map.
func (q UsageStatsQuery) filterMap() map[string]string {
	filters := make(map[string]string, 7)
	for column, value := range map[string]string{
		"provider_uuid": q.Provider,
		"model":         q.Model,
		"scenario":      q.Scenario,
		"rule_uuid":     q.RuleUUID,
		"user_id":       q.UserID,
		"team_id":       q.TeamID,
		"status":        q.Status,
	} {
		if value != "" {
			filters[column] = value
		}
	}
	return filters
}

// GetAggregatedStats returns aggregated statistics. For queries spanning
// several completed days it combines the usage_daily pre-aggregation table
// with a raw scan of only the partial edge days (see usage_daily.go), so
// dashboard loads stay fast regardless of how many raw records accumulate.
func (us *UsageStore) GetAggregatedStats(query UsageStatsQuery) ([]AggregatedStat, error) {
	if stats, ok, err := us.aggregatedStatsFromRollups(query); ok {
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
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64
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
		CacheReadTokens:  b.CacheReadTokens,
		CacheWriteTokens: b.CacheWriteTokens,
		ReasoningTokens:  b.ReasoningTokens,
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

	db := us.db.Model(&UsageRecord{}).Scopes(
		withinTimeRange(query.StartTime, query.EndTime),
		withStatsFilters(query),
	)

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
		COALESCE(SUM(cache_input_tokens), 0) as cache_read_tokens,
		COALESCE(SUM(cache_write_tokens), 0) as cache_write_tokens,
		COALESCE(SUM(reasoning_tokens), 0) as reasoning_tokens,
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
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	ReasoningTokens  int64   `json:"reasoning_tokens"`
	SystemTokens     int64   `json:"system_tokens"`
	ErrorCount       int64   `json:"error_count"`
	AvgLatencyMs     float64 `json:"avg_latency_ms"`
}

// GetTimeSeries returns time-series data for usage. Day-interval queries
// spanning several completed days are served from usage_daily with raw scans
// only for the partial edge days (see usage_daily.go).
func (us *UsageStore) GetTimeSeries(interval string, startTime, endTime time.Time, filters map[string]string) ([]TimeSeriesData, error) {
	if data, ok, err := us.timeSeriesFromRollups(interval, startTime, endTime, filters); ok {
		return data, err
	}
	return us.rawTimeSeries(interval, startTime, endTime, filters)
}

// rawTimeSeries aggregates time buckets directly over usage_records.
func (us *UsageStore) rawTimeSeries(interval string, startTime, endTime time.Time, filters map[string]string) ([]TimeSeriesData, error) {
	buckets, err := us.rawSeriesBuckets(interval, startTime, endTime, filters)
	if err != nil {
		return nil, err
	}
	merged := make(map[string]*aggBucket, len(buckets))
	for _, b := range buckets {
		mergeBucket(merged, b.Key, b)
	}
	return seriesFromBuckets(merged), nil
}

// rawSeriesBuckets groups usage_records into time buckets keyed by the Unix
// seconds of the bucket start.
func (us *UsageStore) rawSeriesBuckets(interval string, startTime, endTime time.Time, filters map[string]string) ([]aggBucket, error) {
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

	if err := validateFilterColumns(filters); err != nil {
		return nil, err
	}
	db := us.db.Model(&UsageRecord{}).Scopes(
		withinTimeRange(startTime, endTime),
		withPinnedRangeFilters(filters),
	)

	// The key is the Unix timestamp of the time bucket (the grouped time), not
	// the original timestamp.
	selectClause := fmt.Sprintf(`
		strftime('%%s', strftime('%s', timestamp)) as key,
		COUNT(*) as request_count,
		COALESCE(SUM(total_tokens), 0) as total_tokens,
		COALESCE(SUM(input_tokens), 0) as input_tokens,
		COALESCE(SUM(output_tokens), 0) as output_tokens,
		COALESCE(SUM(cache_input_tokens), 0) as cache_read_tokens,
		COALESCE(SUM(cache_write_tokens), 0) as cache_write_tokens,
		COALESCE(SUM(reasoning_tokens), 0) as reasoning_tokens,
		COALESCE(SUM(system_tokens), 0) as system_tokens,
		COALESCE(SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END), 0) as error_count,
		COALESCE(SUM(latency_ms), 0) as latency_sum
	`, timeFormat)

	var buckets []aggBucket
	if err := db.
		Select(selectClause).
		Group(fmt.Sprintf("strftime('%s', timestamp)", timeFormat)).
		Scan(&buckets).Error; err != nil {
		return nil, err
	}
	return buckets, nil
}

// GetRecords returns individual usage records (for debugging/audit)
func (us *UsageStore) GetRecords(startTime, endTime time.Time, filters map[string]string, limit, offset int) ([]UsageRecord, int64, error) {
	us.mu.RLock()
	defer us.mu.RUnlock()

	if err := validateFilterColumns(filters); err != nil {
		return nil, 0, err
	}
	base := func() *gorm.DB {
		return us.db.Model(&UsageRecord{}).Scopes(
			withinTimeRange(startTime, endTime),
			withColumnFilters(filters),
		)
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

// MinTPSDecodeWindowMs is the shortest (latency - TTFT) window for which TPS is
// reported. A large first chunk makes a shorter window inflate TPS.
const MinTPSDecodeWindowMs = 1000

// TokensPerSecond derives per-request output TPS from persisted timing fields.
// The first token is accounted for by TTFT, leaving N-1 decode intervals from
// the first output token to the last. Returns 0 when the decode window is
// shorter than MinTPSDecodeWindowMs.
func TokensPerSecond(outputTokens, latencyMs, ttftMs int) float64 {
	decodeMs := latencyMs - ttftMs
	if outputTokens <= 1 || ttftMs <= 0 || decodeMs < MinTPSDecodeWindowMs {
		return 0
	}
	return float64(outputTokens-1) * 1000 / float64(decodeMs)
}

// GetPerformanceSummary calculates percentiles across the complete filtered
// range. Only successful requests participate: failures and cancellations have
// different completion semantics and would distort the user-experience view.
func (us *UsageStore) GetPerformanceSummary(startTime, endTime time.Time, filters map[string]string) (PerformanceSummary, error) {
	if err := validateFilterColumns(filters); err != nil {
		return PerformanceSummary{}, err
	}
	key := perfCacheKey(startTime, endTime, filters, us.perfCacheTTL)
	if cached, ok := us.perf.get(key, us.perfCacheTTL); ok {
		return cached, nil
	}
	gen := us.perf.generation()
	summary, err := us.queryPerformanceSummary(startTime, endTime, filters)
	if err != nil {
		return PerformanceSummary{}, err
	}
	us.perf.put(key, summary, us.perfCacheTTL, gen)
	return summary, nil
}

func (us *UsageStore) queryPerformanceSummary(startTime, endTime time.Time, filters map[string]string) (PerformanceSummary, error) {
	us.mu.RLock()
	defer us.mu.RUnlock()

	// Stream plain ints instead of materializing a UsageRecord per row: this
	// runs over every successful request in the range (hundreds of thousands
	// on a 30-day window), where gorm's per-row reflection dominated.
	rows, err := us.db.Model(&UsageRecord{}).
		Select("latency_ms", "ttft_ms", "output_tokens", "streamed").
		Where("+status = ?", "success").
		Scopes(
			withinTimeRange(startTime, endTime),
			withPinnedRangeFilters(filters),
		).Rows()
	if err != nil {
		return PerformanceSummary{}, err
	}
	defer rows.Close()

	var completion, ttft, tps []float64
	for rows.Next() {
		var latencyMs, ttftMs, outputTokens int
		var streamed bool
		if err := rows.Scan(&latencyMs, &ttftMs, &outputTokens, &streamed); err != nil {
			return PerformanceSummary{}, err
		}
		if latencyMs > 0 {
			completion = append(completion, float64(latencyMs))
		}
		if !streamed {
			continue
		}
		if ttftMs > 0 {
			ttft = append(ttft, float64(ttftMs))
		}
		if speed := TokensPerSecond(outputTokens, latencyMs, ttftMs); speed > 0 {
			tps = append(tps, speed)
		}
	}
	if err := rows.Err(); err != nil {
		return PerformanceSummary{}, err
	}

	return PerformanceSummary{
		TTFT:       summarizePerformanceMetric(ttft),
		TPS:        summarizePerformanceMetric(tps),
		Completion: summarizePerformanceMetric(completion),
	}, nil
}

func summarizePerformanceMetric(values []float64) PerformanceMetricSummary {
	if len(values) == 0 {
		return PerformanceMetricSummary{}
	}
	sort.Float64s(values)
	return PerformanceMetricSummary{
		SampleCount: int64(len(values)),
		P10:         percentileFloat(values, 0.10),
		P50:         percentileFloat(values, 0.50),
		P90:         percentileFloat(values, 0.90),
		P95:         percentileFloat(values, 0.95),
		P99:         percentileFloat(values, 0.99),
	}
}

func percentileFloat(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	index := p * float64(len(sorted)-1)
	lower := int(index)
	upper := lower + 1
	if upper >= len(sorted) {
		return sorted[lower]
	}
	fraction := index - float64(lower)
	return sorted[lower] + fraction*(sorted[upper]-sorted[lower])
}

// DeleteOlderThan deletes records older than the specified date, together
// with the daily aggregates derived from them so both views stay consistent.
func (us *UsageStore) DeleteOlderThan(cutoffDate time.Time) (int64, error) {
	us.mu.Lock()
	defer us.mu.Unlock()
	us.perf.reset()

	var deleted int64
	// One transaction: every rollup table must lose the same buckets, or the
	// one that kept them would never rebuild them.
	err := us.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Where("timestamp < ?", cutoffDate).Delete(&UsageRecord{})
		if res.Error != nil {
			return res.Error
		}
		deleted = res.RowsAffected
		return clearRollupsThrough(tx, cutoffDate)
	})
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

// AggregateToDaily rebuilds the usage_hourly and usage_daily rows for the UTC
// day containing the given time from the raw records, whether or not they
// existed. It returns the number of usage_daily rows the day now has.
func (us *UsageStore) AggregateToDaily(date time.Time) (int64, error) {
	day := utcDayStart(date)
	us.mu.Lock()
	defer us.mu.Unlock()
	var n int64
	err := us.db.Transaction(func(tx *gorm.DB) error {
		if err := buildHours(tx, day, day.Add(24*time.Hour)); err != nil {
			return err
		}
		if err := deriveDays(tx, day, day.Add(24*time.Hour)); err != nil {
			return err
		}
		return tx.Model(&UsageDailyRecord{}).Where("date = ?", dayTier.key(day)).Count(&n).Error
	})
	return n, err
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
