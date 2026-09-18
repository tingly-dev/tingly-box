package ops

import "github.com/tingly-dev/tingly-box/internal/protocol/metaid"

// The metadata.user_id model lives in the leaf package metaid so the protocol
// converters can read a request's session identity too (ops sits above them and
// cannot be imported from there). These aliases keep ops' long-standing names
// working for its own call sites.

// MetadataUserID represents the JSON structure for metadata.user_id.
type MetadataUserID = metaid.MetadataUserID

// ParseMetadataUserID parses a metadata.user_id string in either JSON or legacy format.
func ParseMetadataUserID(raw string) *MetadataUserID { return metaid.ParseMetadataUserID(raw) }

// BuildMetadataUserID builds a MetadataUserID from an extra map.
func BuildMetadataUserID(extra map[string]any) *MetadataUserID {
	return metaid.BuildMetadataUserID(extra)
}

// FixMetadataUserID parses and fixes a metadata user ID string.
func FixMetadataUserID(raw string) *MetadataUserID { return metaid.FixMetadataUserID(raw) }

// FormatMetadataUserID formats a MetadataUserID to a JSON string.
func FormatMetadataUserID(m *MetadataUserID) string { return metaid.FormatMetadataUserID(m) }
