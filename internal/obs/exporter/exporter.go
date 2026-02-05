package exporter

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// MultiExporter aggregates multiple metric exporters and exports to all of them.
// Errors from individual exporters are logged but do not prevent other exporters from running.
type MultiExporter struct {
	exporters []metric.Exporter
	mu        sync.Mutex
}

// NewMultiExporter creates a new MultiExporter with the provided exporters.
func NewMultiExporter(exporters ...metric.Exporter) *MultiExporter {
	return &MultiExporter{
		exporters: exporters,
	}
}

// Temporality returns the Temporality to use for an instrument kind.
func (m *MultiExporter) Temporality(kind metric.InstrumentKind) metricdata.Temporality {
	// Return default temporality (cumulative)
	return metricdata.CumulativeTemporality
}

// Aggregation returns the Aggregation to use for an instrument kind.
func (m *MultiExporter) Aggregation(kind metric.InstrumentKind) metric.Aggregation {
	// Return default aggregation
	return metric.DefaultAggregationSelector(kind)
}

// Export exports the resource metrics to all registered exporters.
func (m *MultiExporter) Export(ctx context.Context, res *metricdata.ResourceMetrics) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Export to all registered exporters
	// We collect errors but continue with other exporters
	var firstErr error
	for _, e := range m.exporters {
		if err := e.Export(ctx, res); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// ForceFlush forces all exporters to flush any pending data.
func (m *MultiExporter) ForceFlush(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, e := range m.exporters {
		_ = e.ForceFlush(ctx)
	}
	return nil
}

// Shutdown shuts down all exporters.
func (m *MultiExporter) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, e := range m.exporters {
		_ = e.Shutdown(ctx)
	}
	return nil
}
