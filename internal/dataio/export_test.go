package dataio

import (
	"testing"
)

// TestExporterFormat verifies that the exporter constructors return an
// exporter whose Format() matches the requested format, and that an invalid
// format is rejected. The trivial constant/non-nil checks have been folded
// into this single behavioral test.
func TestExporterFormat(t *testing.T) {
	tests := []struct {
		name    string
		format  Format
		wantErr bool
	}{
		{"JSONL exporter", FormatJSONL, false},
		{"Base64 exporter", FormatBase64, false},
		{"Invalid format", Format("invalid"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter, err := NewExporter(tt.format)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewExporter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && exporter.Format() != tt.format {
				t.Errorf("NewExporter() format = %v, want %v", exporter.Format(), tt.format)
			}
		})
	}
}
