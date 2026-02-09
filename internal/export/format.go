package export

// Format represents the export format type
type Format string

const (
	// FormatJSONL is the line-delimited JSON format
	FormatJSONL Format = "jsonl"
	// FormatBase64 is the Base64-encoded JSONL format
	FormatBase64 Format = "base64"
)

const (
	// Base64Prefix is the prefix for Base64 format exports
	Base64Prefix = "TGB64"
	// CurrentVersion is the current export format version
	CurrentVersion = "1.0"
)

// ExportResult represents the result of an export operation
type ExportResult struct {
	Format  Format
	Content string
}
