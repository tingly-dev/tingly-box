package dataimport

import (
	"strings"
)

// Detector detects the format of import data
type Detector struct{}

// NewDetector creates a new format detector
func NewDetector() *Detector {
	return &Detector{}
}

// Detect detects the format of the input data
func (d *Detector) Detect(data string) Format {
	// Trim whitespace
	data = strings.TrimSpace(data)

	// Check for Base64 prefix
	if strings.HasPrefix(data, Base64Prefix+":") {
		return FormatBase64
	}

	// Default to JSONL for backward compatibility
	return FormatJSONL
}
