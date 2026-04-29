package exporter

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/tingly-dev/tingly-box/internal/obs"
)

// SinkExporter exports OTel metrics to the recording sink as *obs.Record entries.
type SinkExporter struct {
	sink *obs.Sink
}

// NewSinkExporter creates a new SinkExporter.
func NewSinkExporter(sink *obs.Sink) *SinkExporter {
	return &SinkExporter{sink: sink}
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
	for _, scopeMetrics := range res.ScopeMetrics {
		for _, metricData := range scopeMetrics.Metrics {
			e.exportMetric(metricData)
		}
	}
	return nil
}

func (e *SinkExporter) exportMetric(m metricdata.Metrics) {
	switch data := m.Data.(type) {
	case metricdata.Sum[int64]:
		for _, dp := range data.DataPoints {
			attrs := dp.Attributes
			provider := extractAttr(attrs, "llm.provider")
			model := extractAttr(attrs, "llm.model")
			scenario := extractAttr(attrs, "llm.scenario")

			r := &obs.Record{
				Timestamp: time.Now().UTC(),
				Provider:  provider,
				Scenario:  scenario,
				Model:     model,
			}
			e.sink.Emit(r)
		}
	}
}

// ForceFlush forces a flush of pending data.
func (e *SinkExporter) ForceFlush(ctx context.Context) error { return nil }

// Shutdown shuts down the exporter.
func (e *SinkExporter) Shutdown(ctx context.Context) error { return nil }
