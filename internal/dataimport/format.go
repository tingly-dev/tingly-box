package dataimport

// Format represents the import format type
type Format string

const (
	// FormatAuto automatically detects the format from input data
	FormatAuto Format = "auto"
	// FormatJSONL is the line-delimited JSON format
	FormatJSONL Format = "jsonl"
	// FormatBase64 is the Base64-encoded JSONL format
	FormatBase64 Format = "base64"
)

const (
	// Base64Prefix is the prefix for Base64 format imports
	Base64Prefix = "TGB64"
)
