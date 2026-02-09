package export

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// Base64Exporter exports data in Base64-encoded JSONL format
type Base64Exporter struct {
	jsonlExporter *JSONLExporter
}

// NewBase64Exporter creates a new Base64 exporter
func NewBase64Exporter() *Base64Exporter {
	return &Base64Exporter{
		jsonlExporter: NewJSONLExporter(),
	}
}

// Export performs the export in Base64 format
func (e *Base64Exporter) Export(req *ExportRequest) (*ExportResult, error) {
	if req.Rule == nil {
		return nil, fmt.Errorf("rule is required for export")
	}

	// First, get JSONL content
	jsonlResult, err := e.jsonlExporter.Export(req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JSONL: %w", err)
	}

	// Encode JSONL content as Base64
	encoded := base64.StdEncoding.EncodeToString([]byte(jsonlResult.Content))

	// Add prefix
	content := fmt.Sprintf("%s:%s:%s", Base64Prefix, CurrentVersion, encoded)

	return &ExportResult{
		Format:  FormatBase64,
		Content: content,
	}, nil
}

// Format returns the format type
func (e *Base64Exporter) Format() Format {
	return FormatBase64
}

// DecodeBase64Export decodes a Base64 export back to JSONL content
func DecodeBase64Export(data string) (string, error) {
	// Remove whitespace
	data = strings.TrimSpace(data)

	// Check prefix
	if !strings.HasPrefix(data, Base64Prefix+":") {
		return "", fmt.Errorf("invalid Base64 export format: missing %s prefix", Base64Prefix)
	}

	// Find and extract version and payload
	parts := strings.SplitN(data, ":", 3)
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid Base64 export format: expected prefix:version:payload")
	}

	version := parts[1]
	payload := parts[2]

	// Validate version
	if version != CurrentVersion {
		return "", fmt.Errorf("unsupported version: %s (supported: %s)", version, CurrentVersion)
	}

	// Decode Base64
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", fmt.Errorf("failed to decode Base64: %w", err)
	}

	return string(decoded), nil
}
