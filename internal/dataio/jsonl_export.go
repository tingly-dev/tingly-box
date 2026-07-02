package dataio

import (
	"encoding/json"
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ExportRequest contains the data needed for export
type ExportRequest struct {
	Providers []*typ.Provider
}

// JSONLExporter exports data in JSONL format
type JSONLExporter struct{}

// NewJSONLExporter creates a new JSONL exporter
func NewJSONLExporter() *JSONLExporter {
	return &JSONLExporter{}
}

// Export performs the export in JSONL format
func (e *JSONLExporter) Export(req *ExportRequest) (*ExportResult, error) {
	if len(req.Providers) == 0 {
		return nil, fmt.Errorf("providers must be specified for export")
	}

	lines, err := e.buildJSONLLines(req)
	if err != nil {
		return nil, fmt.Errorf("failed to build JSONL: %w", err)
	}

	return &ExportResult{
		Format:  FormatJSONL,
		Content: lines,
	}, nil
}

// Format returns the format type
func (e *JSONLExporter) Format() Format {
	return FormatJSONL
}

// buildJSONLLines constructs the JSONL content from providers
func (e *JSONLExporter) buildJSONLLines(req *ExportRequest) (string, error) {
	lines := make([]string, 0, 1+len(req.Providers))

	// Line 1: Metadata
	metadata := Metadata{
		Type:       "metadata",
		Version:    CurrentVersion,
		ExportedAt: timestamp(),
	}
	metadataLine, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("failed to marshal metadata: %w", err)
	}
	lines = append(lines, string(metadataLine))

	// Subsequent lines: Providers
	for _, provider := range req.Providers {
		providerData := e.buildProviderData(provider)
		providerLine, err := json.Marshal(providerData)
		if err != nil {
			return "", fmt.Errorf("failed to marshal provider: %w", err)
		}
		lines = append(lines, string(providerLine))
	}

	return joinLines(lines), nil
}

// buildProviderData converts a Provider to ProviderData. Embedding means this
// is a straight copy — no field list to keep in sync with ai.Provider.
func (e *JSONLExporter) buildProviderData(provider *typ.Provider) ProviderData {
	return ProviderData{
		Type:     "provider",
		Provider: *provider,
	}
}
