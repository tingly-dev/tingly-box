package exporter

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/tingly-dev/tingly-box/internal/obs"
)

// SinkExporter exports metrics to the JSONL file sink.
// It maintains compatibility with the existing RecordEntry format.
type SinkExporter struct {
	sink *obs.Sink
}

// NewSinkExporter creates a new SinkExporter.
func NewSinkExporter(sink *obs.Sink) *SinkExporter {
	return &SinkExporter{
		sink: sink,
	}
}

// Temporality returns the Temporality to use for an instrument kind.
func (e *SinkExporter) Temporality(kind metric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}

// Aggregation returns the Aggregation to use for an instrument kind.
func (e *SinkExporter) Aggregation(kind metric.InstrumentKind) metric.Aggregation {
	return metric.DefaultAggregationSelector(kind)
}

// Export exports metrics to the sink.
func (e *SinkExporter) Export(ctx context.Context, res *metricdata.ResourceMetrics) error {
	if e.sink == nil || !e.sink.IsEnabled() {
		return nil
	}

	// Aggregate metrics into usage records
	for _, scopeMetrics := range res.ScopeMetrics {
		for _, metricData := range scopeMetrics.Metrics {
			e.convertToRecordEntry(metricData)
		}
	}

	return nil
}

// convertToRecordEntry converts a metric to a RecordEntry and writes to the sink.
func (e *SinkExporter) convertToRecordEntry(metricData metricdata.Metrics) {
	// The sink records full request/response pairs, metrics are summary data
	// We create a summary record entry for metric exports
	switch data := metricData.Data.(type) {
	case metricdata.Sum[int64]:
		e.processSumForSink(data, metricData)
	}
}

// processSumForSink processes a sum metric for the sink.
func (e *SinkExporter) processSumForSink(data metricdata.Sum[int64], metricData metricdata.Metrics) {
	for _, dp := range data.DataPoints {
		attrs := dp.Attributes
		value := dp.Value

		// Extract attributes
		provider := extractAttr(attrs, "llm.provider")
		model := extractAttr(attrs, "llm.model")
		scenario := extractAttr(attrs, "llm.scenario")

		// Create a metadata record entry
		metadata := map[string]interface{}{
			"metric_name": metricData.Name,
			"metric_type": "sum",
			"attributes":  attrsToMap(attrs),
		}

		// Create a record entry for the metric
		entry := &obs.RecordEntry{
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			RequestID:  uuid.New().String(),
			Provider:   provider,
			Scenario:   scenario,
			Model:      model,
			DurationMs: 0,
			Metadata:   metadata,
		}

		// Add the metric value to metadata
		if jsonBytes, err := json.Marshal(value); err == nil {
			var rawValue json.RawMessage
			rawValue = jsonBytes
			metadata["value"] = &rawValue
		}

		// Write to the sink
		e.writeToSink(provider, scenario, entry)
	}
}

// writeToSink writes an entry to the sink using the appropriate scenario.
func (e *SinkExporter) writeToSink(provider, scenario string, entry *obs.RecordEntry) {
	// Use RecordWithScenario to maintain file naming convention
	e.sink.RecordWithScenario(provider, entry.Model, scenario, nil, nil, 0, nil)
}

// ForceFlush forces a flush of pending data.
func (e *SinkExporter) ForceFlush(ctx context.Context) error {
	return nil
}

// Shutdown shuts down the exporter.
func (e *SinkExporter) Shutdown(ctx context.Context) error {
	return nil
}

// attrsToMap converts an attribute set to a map.
func attrsToMap(attrs attribute.Set) map[string]string {
	result := make(map[string]string)
	iter := attrs.Iter()
	for iter.Next() {
		kv := iter.Attribute()
		result[string(kv.Key)] = kv.Value.Emit()
	}
	return result
}
