package export

import (
	"fmt"
	"time"
)

// Exporter defines the interface for export implementations
type Exporter interface {
	Export(req *ExportRequest) (*ExportResult, error)
	Format() Format
}

// NewExporter creates an exporter for the specified format
func NewExporter(format Format) (Exporter, error) {
	switch format {
	case FormatJSONL:
		return NewJSONLExporter(), nil
	case FormatBase64:
		return NewBase64Exporter(), nil
	default:
		return nil, fmt.Errorf("unsupported export format: %s", format)
	}
}

// Export exports a rule with its providers in the specified format
func Export(req *ExportRequest, format Format) (*ExportResult, error) {
	exporter, err := NewExporter(format)
	if err != nil {
		return nil, err
	}
	return exporter.Export(req)
}

// timestamp returns the current time in ISO 8601 format
func timestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// joinLines joins strings with newline separators
func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	result := lines[0]
	for i := 1; i < len(lines); i++ {
		result += "\n" + lines[i]
	}
	return result
}
