package dataimport

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/server/config"
)

// Base64Importer imports data from Base64-encoded JSONL format
type Base64Importer struct {
	jsonlImporter *JSONLImporter
}

// NewBase64Importer creates a new Base64 importer
func NewBase64Importer() *Base64Importer {
	return &Base64Importer{
		jsonlImporter: NewJSONLImporter(),
	}
}

// Format returns the format type
func (i *Base64Importer) Format() Format {
	return FormatBase64
}

// Import imports data from Base64 format
func (i *Base64Importer) Import(data string, globalConfig *config.Config, opts ImportOptions) (*ImportResult, error) {
	// Decode Base64 to JSONL
	jsonlData, err := i.decodeBase64Export(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode Base64 export: %w", err)
	}

	// Use JSONL importer to process the decoded data
	return i.jsonlImporter.Import(jsonlData, globalConfig, opts)
}

// decodeBase64Export decodes a Base64 export back to JSONL content
func (i *Base64Importer) decodeBase64Export(data string) (string, error) {
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
	if version != "1.0" {
		return "", fmt.Errorf("unsupported version: %s (supported: 1.0)", version)
	}

	// Decode Base64
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", fmt.Errorf("failed to decode Base64: %w", err)
	}

	return string(decoded), nil
}
