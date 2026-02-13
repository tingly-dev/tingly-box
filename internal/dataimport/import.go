package dataimport

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/server/config"
)

// Import imports a rule with providers from data in the specified format
func Import(data string, globalConfig *config.Config, format Format, opts ImportOptions) (*ImportResult, error) {
	importer, err := NewImporter(format)
	if err != nil {
		return nil, err
	}

	// Auto-detect format if needed
	if format == FormatAuto {
		detector := NewDetector()
		detectedFormat := detector.Detect(data)
		importer, err = NewImporter(detectedFormat)
		if err != nil {
			return nil, err
		}
	}

	return importer.Import(data, globalConfig, opts)
}

// NewImporter creates an importer for the specified format
func NewImporter(format Format) (Importer, error) {
	switch format {
	case FormatJSONL:
		return NewJSONLImporter(), nil
	case FormatBase64:
		return NewBase64Importer(), nil
	case FormatAuto:
		return NewJSONLImporter(), nil // Default to JSONL, will be overridden in Import()
	default:
		return nil, fmt.Errorf("unsupported import format: %s", format)
	}
}
