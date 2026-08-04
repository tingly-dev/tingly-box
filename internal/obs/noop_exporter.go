package obs

import "context"

// noopExporter is a sink exporter used by experiments/tests that need a valid
// Sink without writing records anywhere.
type noopExporter struct{}

// NewNoopExporter returns a RecordExporter that discards all records.
func NewNoopExporter() RecordExporter {
	return noopExporter{}
}

func (noopExporter) Export(context.Context, []*Record) error { return nil }
func (noopExporter) Shutdown(context.Context) error          { return nil }
