package ops

import (
	"encoding/json"
	"regexp"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// =============================================
// Metadata User ID Structures
// =============================================

// MetadataUserID represents the JSON structure for metadata.user_id
// This matches Claude Code's format (>= 2.1.78)
type MetadataUserID struct {
	DeviceID    string `json:"device_id"`
	AccountUUID string `json:"account_uuid"`
	SessionID   string `json:"session_id"`
}

// legacyUserIDRegex matches the legacy user_id format:
//
//	user_{64hex}_account_{optional_uuid}_session_{uuid}
var legacyUserIDRegex = regexp.MustCompile(`^user_([a-fA-F0-9]{64})_account_([a-fA-F0-9-]*)_session_([a-fA-F0-9-]{36})$`)

// =============================================
// Parsing Functions
// =============================================

// ParseMetadataUserID parses a metadata.user_id string in either JSON or legacy format.
// Returns nil if the input cannot be parsed.
func ParseMetadataUserID(raw string) *MetadataUserID {
	if raw == "" {
		return nil
	}

	// Try JSON format first
	var jsonResult MetadataUserID
	if err := json.Unmarshal([]byte(raw), &jsonResult); err == nil {
		return &jsonResult
	}

	// Try legacy format: user_{64hex}_account_{optional_uuid}_session_{uuid}
	matches := legacyUserIDRegex.FindStringSubmatch(raw)
	if len(matches) == 4 {
		return &MetadataUserID{
			DeviceID:    matches[1],
			AccountUUID: matches[2],
			SessionID:   matches[3],
		}
	}

	return nil
}

// Format converts MetadataUserID to string for metadata.
func (m *MetadataUserID) Format() string {
	// legacy format
	//return fmt.Sprintf("user_%s_account_%s_session_%s", m.DeviceID, m.AccountUUID, m.SessionID)

	// new json format
	s, err := json.Marshal(m)
	if err != nil {
		logrus.Errorf("MetadataUserID.Format: error marshalling MetadataUserID: %v", err)
	}
	return string(s)
}

// =============================================
// Builder Functions
// =============================================

func (m *MetadataUserID) Fix(extras map[string]any) {
	// Generate default device_id if not set (use extras UUID as identifier)
	if extras != nil {
		if v, ok := extras["device"]; ok {
			m.DeviceID = v.(string)
		}
	}

	// force to guard
	if m.DeviceID == "" {
		panic("missing device id")
	}

	// Set account id if given
	if extras != nil {
		if v, ok := extras["user_id"]; ok {
			m.AccountUUID = v.(string)
		}
	}

	if m.AccountUUID == "" {
		panic("missing account uuid")
	}

	// Ensure session_id is set
	if m.SessionID == "" {
		m.SessionID = uuid.New().String()
	}
}

// =============================================
// Validation Functions
// =============================================

// IsValid checks if the MetadataUserID has required fields.
func (m *MetadataUserID) IsValid() bool {
	if m == nil {
		return false
	}
	return m.DeviceID != "" && m.SessionID != ""
}

// IsEmpty checks if the MetadataUserID is effectively empty.
func (m *MetadataUserID) IsEmpty() bool {
	if m == nil {
		return true
	}
	return m.DeviceID == "" && m.AccountUUID == "" && m.SessionID == ""
}

// =============================================
// Helper Functions
// =============================================

// BuildMetadataUserID builds a MetadataUserID from extra map.
// Returns nil if all fields are empty after fixing.
func BuildMetadataUserID(extra map[string]any) *MetadataUserID {
	m := &MetadataUserID{}

	// Always call Fix to generate default values if needed
	m.Fix(extra)

	// Only return nil if all fields are empty after fixing
	if m.IsEmpty() {
		return nil
	}

	return m
}

// FixMetadataUserID parses and fixes a metadata user ID string.
// Returns a new MetadataUserID with generated fields for missing values.
func FixMetadataUserID(raw string) *MetadataUserID {
	m := ParseMetadataUserID(raw)
	if m == nil {
		m = &MetadataUserID{}
	}
	m.Fix(nil)
	return m
}

// FormatMetadataUserID formats a MetadataUserID to JSON string.
// Returns empty string for nil input.
func FormatMetadataUserID(m *MetadataUserID) string {
	if m == nil {
		return ""
	}
	return m.Format()
}
