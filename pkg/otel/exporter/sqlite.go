package exporter

import (
	"context"

	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/tingly-dev/tingly-box/internal/data/db"
)

// SQLiteExporter exports metrics to the existing SQLite database.
// It maintains compatibility with the existing UsageRecord schema.
type SQLiteExporter struct {
	usageStore *db.UsageStore
}

// NewSQLiteExporter creates a new SQLiteExporter.
func NewSQLiteExporter(usageStore *db.UsageStore) *SQLiteExporter {
	return &SQLiteExporter{
		usageStore: usageStore,
	}
}

// Temporality returns the Temporality to use for an instrument kind.
func (e *SQLiteExporter) Temporality(kind metric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}

// Aggregation returns the Aggregation to use for an instrument kind.
func (e *SQLiteExporter) Aggregation(kind metric.InstrumentKind) metric.Aggregation {
	return metric.DefaultAggregationSelector(kind)
}

// Export exports metrics to SQLite.
func (e *SQLiteExporter) Export(ctx context.Context, res *metricdata.ResourceMetrics) error {
	for _, scopeMetrics := range res.ScopeMetrics {
		for _, metricData := range scopeMetrics.Metrics {
			e.processMetric(metricData)
		}
	}
	return nil
}

// processMetric processes a single metric and writes to the appropriate store.
func (e *SQLiteExporter) processMetric(metricData metricdata.Metrics) {
	switch data := metricData.Data.(type) {
	case metricdata.Sum[int64]:
		e.processSum(data, metricData)
	case metricdata.Sum[float64]:
		// Not currently used, but handle for completeness
	case metricdata.Histogram[float64]:
		e.processHistogram(data, metricData)
	case metricdata.Histogram[int64]:
		e.processHistogramInt64(data, metricData)
	}
}

// processSum processes a sum metric (e.g., token counters, request counters).
func (e *SQLiteExporter) processSum(data metricdata.Sum[int64], metricData metricdata.Metrics) {
	for _, dp := range data.DataPoints {
		attrs := dp.Attributes
		value := dp.Value

		// Extract attributes
		provider := extractAttr(attrs, "llm.provider")
		model := extractAttr(attrs, "llm.model")
		scenario := extractAttr(attrs, "llm.scenario")
		status := extractAttr(attrs, "llm.response.status")
		providerUUID := extractAttr(attrs, "llm.provider.uuid")
		ruleUUID := extractAttr(attrs, "llm.rule.uuid")

		// Route to appropriate store based on metric name
		// Support both new metric names (pkg/otel) for compatibility
		switch metricData.Name {
		case "llm.token.usage.input":
			e.recordTokenUsage(provider, providerUUID, model, ruleUUID, scenario, "input", value, status)
		case "llm.token.usage.output":
			e.recordTokenUsage(provider, providerUUID, model, ruleUUID, scenario, "output", value, status)
		case "llm.token.total":
			// Total tokens handled via usage store
		case "llm.request.count":
			e.recordRequestCount(provider, providerUUID, model, ruleUUID, scenario, status, value)
		case "llm.request.errors":
			// Errors are tracked via status in usage records
		}
	}
}

// processHistogram processes a histogram metric (e.g., request duration).
func (e *SQLiteExporter) processHistogram(data metricdata.Histogram[float64], metricData metricdata.Metrics) {
	// Request duration is tracked via usage records, no separate histogram storage
}

// processHistogramInt64 processes an int64 histogram metric.
func (e *SQLiteExporter) processHistogramInt64(data metricdata.Histogram[int64], metricData metricdata.Metrics) {
	// Request duration is tracked via usage records, no separate histogram storage
}

// recordTokenUsage records token usage to the usage store.
// Note: This is a placeholder for OTel SDK compatibility. Actual recording happens
// in internal/server/usage_tracking.go via the TokenTracker, which writes to the
// UsageStore directly. The SQLite exporter is primarily for metric collection,
// while the detailed records are written through the tracking layer.
func (e *SQLiteExporter) recordTokenUsage(provider, providerUUID, model, ruleUUID, scenario, tokenType string, tokens int64, status string) {
	// Actual recording happens in usage_tracking.go via RecordUsage
	// This exporter is primarily for OTel SDK compatibility
}

// recordRequestCount records a request count to the database.
// Note: This is a placeholder for OTel SDK compatibility. Request counts are
// tracked via usage records in internal/server/tracking.go.
func (e *SQLiteExporter) recordRequestCount(provider, providerUUID, model, ruleUUID, scenario, status string, count int64) {
	// Request counts are tracked via usage records in tracking.go
}

// ForceFlush forces a flush of pending data.
func (e *SQLiteExporter) ForceFlush(ctx context.Context) error {
	// SQLite writes are synchronous, no pending data
	return nil
}

// Shutdown shuts down the exporter.
func (e *SQLiteExporter) Shutdown(ctx context.Context) error {
	return nil
}
