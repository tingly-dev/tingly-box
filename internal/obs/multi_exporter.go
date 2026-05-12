package obs

import (
	"context"

	"github.com/sirupsen/logrus"
)

// MultiExporter fans Export and Shutdown calls out to multiple RecordExporters.
// One exporter failing does not prevent the others from running; the first
// non-nil error is returned.
type MultiExporter struct {
	exporters []RecordExporter
}

// NewMultiExporter aggregates the given exporters behind a single
// RecordExporter interface. Nil exporters are skipped.
func NewMultiExporter(exporters ...RecordExporter) *MultiExporter {
	live := make([]RecordExporter, 0, len(exporters))
	for _, e := range exporters {
		if e != nil {
			live = append(live, e)
		}
	}
	return &MultiExporter{exporters: live}
}

// Export forwards records to every registered exporter.
func (m *MultiExporter) Export(ctx context.Context, records []*Record) error {
	var firstErr error
	for _, e := range m.exporters {
		if err := e.Export(ctx, records); err != nil {
			logrus.Warnf("obs: exporter %T: %v", e, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// Shutdown closes every registered exporter.
func (m *MultiExporter) Shutdown(ctx context.Context) error {
	var firstErr error
	for _, e := range m.exporters {
		if err := e.Shutdown(ctx); err != nil {
			logrus.Warnf("obs: exporter %T shutdown: %v", e, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
