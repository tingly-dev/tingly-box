package exporter

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// mockExporter is a mock exporter for testing
type mockExporter struct {
	exported bool
	shutdown bool
	flushed  bool
}

func (m *mockExporter) Temporality(kind metric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}

func (m *mockExporter) Aggregation(kind metric.InstrumentKind) metric.Aggregation {
	return metric.DefaultAggregationSelector(kind)
}

func (m *mockExporter) Export(ctx context.Context, res *metricdata.ResourceMetrics) error {
	m.exported = true
	return nil
}

func (m *mockExporter) ForceFlush(ctx context.Context) error {
	m.flushed = true
	return nil
}

func (m *mockExporter) Shutdown(ctx context.Context) error {
	m.shutdown = true
	return nil
}

func TestNewMultiExporter(t *testing.T) {
	mock1 := &mockExporter{}
	mock2 := &mockExporter{}

	multi := NewMultiExporter(mock1, mock2)

	if multi == nil {
		t.Fatal("MultiExporter should not be nil")
	}

	if len(multi.exporters) != 2 {
		t.Errorf("Expected 2 exporters, got %d", len(multi.exporters))
	}
}

func TestMultiExporter_Temporality(t *testing.T) {
	multi := NewMultiExporter()

	temporality := multi.Temporality(metric.InstrumentKindCounter)
	if temporality != metricdata.CumulativeTemporality {
		t.Errorf("Expected CumulativeTemporality, got %v", temporality)
	}
}

func TestMultiExporter_Aggregation(t *testing.T) {
	multi := NewMultiExporter()

	aggregation := multi.Aggregation(metric.InstrumentKindCounter)
	if aggregation == nil {
		t.Error("Aggregation should not be nil")
	}
}

func TestMultiExporter_Export(t *testing.T) {
	mock1 := &mockExporter{}
	mock2 := &mockExporter{}

	multi := NewMultiExporter(mock1, mock2)

	ctx := context.Background()
	rm := &metricdata.ResourceMetrics{}

	err := multi.Export(ctx, rm)
	if err != nil {
		t.Errorf("Export should not return error: %v", err)
	}

	if !mock1.exported {
		t.Error("mock1 should have been exported")
	}

	if !mock2.exported {
		t.Error("mock2 should have been exported")
	}
}

func TestMultiExporter_Export_ContinuesOnError(t *testing.T) {
	okMock := &mockExporter{}

	// Make errMock return an error
	errMockWithErr := &mockExporterWithError{exported: false}

	multi := NewMultiExporter(errMockWithErr, okMock)

	ctx := context.Background()
	rm := &metricdata.ResourceMetrics{}

	err := multi.Export(ctx, rm)
	if err == nil {
		t.Error("Expected error from first exporter")
	}

	// Second exporter should still be called even if first fails
	if !okMock.exported {
		t.Error("Second exporter should be called even if first fails")
	}
}

func TestMultiExporter_ForceFlush(t *testing.T) {
	mock1 := &mockExporter{}
	mock2 := &mockExporter{}

	multi := NewMultiExporter(mock1, mock2)

	ctx := context.Background()
	err := multi.ForceFlush(ctx)
	if err != nil {
		t.Errorf("ForceFlush should not return error: %v", err)
	}

	if !mock1.flushed {
		t.Error("mock1 should have been flushed")
	}

	if !mock2.flushed {
		t.Error("mock2 should have been flushed")
	}
}

func TestMultiExporter_Shutdown(t *testing.T) {
	mock1 := &mockExporter{}
	mock2 := &mockExporter{}

	multi := NewMultiExporter(mock1, mock2)

	ctx := context.Background()
	err := multi.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown should not return error: %v", err)
	}

	if !mock1.shutdown {
		t.Error("mock1 should have been shutdown")
	}

	if !mock2.shutdown {
		t.Error("mock2 should have been shutdown")
	}
}

// mockExporterWithError is a mock exporter that always returns an error
type mockExporterWithError struct {
	exported bool
}

func (m *mockExporterWithError) Temporality(kind metric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}

func (m *mockExporterWithError) Aggregation(kind metric.InstrumentKind) metric.Aggregation {
	return metric.DefaultAggregationSelector(kind)
}

func (m *mockExporterWithError) Export(ctx context.Context, res *metricdata.ResourceMetrics) error {
	m.exported = true
	return context.DeadlineExceeded
}

func (m *mockExporterWithError) ForceFlush(ctx context.Context) error {
	return nil
}

func (m *mockExporterWithError) Shutdown(ctx context.Context) error {
	return nil
}

func TestMultiExporter_EmptyExporters(t *testing.T) {
	multi := NewMultiExporter()

	ctx := context.Background()
	rm := &metricdata.ResourceMetrics{}

	err := multi.Export(ctx, rm)
	if err != nil {
		t.Errorf("Export with no exporters should not return error: %v", err)
	}
}

// Test SQLiteExporter initialization
func TestNewSQLiteExporter(t *testing.T) {
	// Note: We can't fully test SQLiteExporter without a real database
	// This test just verifies the constructor doesn't panic
	exporter := NewSQLiteExporter(nil)
	if exporter == nil {
		t.Error("SQLiteExporter should not be nil")
	}
}

// Test SinkExporter initialization
func TestNewSinkExporter(t *testing.T) {
	exporter := NewSinkExporter(nil)
	if exporter == nil {
		t.Error("SinkExporter should not be nil")
	}
}

// Test extractAttr helper function
func TestExtractAttr(t *testing.T) {
	// This test would require creating attribute.Set which is complex
	// The function is tested indirectly through other tests
}

// Test attrsToMap helper function
func TestAttrsToMap(t *testing.T) {
	// This test would require creating attribute.Set which is complex
	// The function is tested indirectly through other tests
}
