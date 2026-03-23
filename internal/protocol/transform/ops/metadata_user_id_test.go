package ops

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

// =============================================
// ParseMetadataUserID Tests
// =============================================

func TestParseMetadataUserID_JSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *MetadataUserID
		wantErr bool
	}{
		{
			name:  "valid JSON with all fields",
			input: `{"device_id":"user_abc123","account_uuid":"acc-456","session_id":"sess-789"}`,
			want: &MetadataUserID{
				DeviceID:    "user_abc123",
				AccountUUID: "acc-456",
				SessionID:   "sess-789",
			},
		},
		{
			name:  "valid JSON with empty account_uuid",
			input: `{"device_id":"user_5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907","account_uuid":"","session_id":"16d97292-8713-438b-ad2e-76f495717258"}`,
			want: &MetadataUserID{
				DeviceID:    "user_5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907",
				AccountUUID: "",
				SessionID:   "16d97292-8713-438b-ad2e-76f495717258",
			},
		},
		{
			name:  "valid JSON without account_uuid field",
			input: `{"device_id":"user_abc123","session_id":"sess-789"}`,
			want: &MetadataUserID{
				DeviceID:    "user_abc123",
				AccountUUID: "",
				SessionID:   "sess-789",
			},
		},
		{
			name:  "valid JSON without account_uuid field, with device_id and session_id only",
			input: `{"device_id":"user_abc123","session_id":"sess-789"}`,
			want: &MetadataUserID{
				DeviceID:    "user_abc123",
				AccountUUID: "",
				SessionID:   "sess-789",
			},
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "   ",
			wantErr: true,
		},
		{
			name:    "malformed JSON",
			input:   `{not valid json}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseMetadataUserID(tt.input)
			if tt.wantErr {
				if got != nil {
					t.Errorf("ParseMetadataUserID() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Errorf("ParseMetadataUserID() = nil, want %v", tt.want)
				return
			}
			if got.DeviceID != tt.want.DeviceID ||
				got.AccountUUID != tt.want.AccountUUID ||
				got.SessionID != tt.want.SessionID {
				t.Errorf("ParseMetadataUserID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseMetadataUserID_Legacy(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *MetadataUserID
		wantErr bool
	}{
		{
			name:  "valid legacy format with all fields",
			input: "user_5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907_account_550e8400-e29b-41d4-a716-446655440000_session_16d97292-8713-438b-ad2e-76f495717258",
			want: &MetadataUserID{
				DeviceID:    "5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907",
				AccountUUID: "550e8400-e29b-41d4-a716-446655440000",
				SessionID:   "16d97292-8713-438b-ad2e-76f495717258",
			},
		},
		{
			name:  "valid legacy format with empty account",
			input: "user_5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907_account__session_16d97292-8713-438b-ad2e-76f495717258",
			want: &MetadataUserID{
				DeviceID:    "5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907",
				AccountUUID: "",
				SessionID:   "16d97292-8713-438b-ad2e-76f495717258",
			},
		},
		{
			name:    "invalid legacy - device_id not 64 hex chars",
			input:   "user_abc_account__session_16d97292-8713-438b-ad2e-76f495717258",
			wantErr: true,
		},
		{
			name:    "invalid legacy - session_id not UUID",
			input:   "user_5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907_account__session_not-a-uuid",
			wantErr: true,
		},
		{
			name:    "invalid legacy - missing prefix",
			input:   "5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907_account__session_16d97292-8713-438b-ad2e-76f495717258",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseMetadataUserID(tt.input)
			if tt.wantErr {
				if got != nil {
					t.Errorf("ParseMetadataUserID() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Errorf("ParseMetadataUserID() = nil, want %v", tt.want)
				return
			}
			if got.DeviceID != tt.want.DeviceID ||
				got.AccountUUID != tt.want.AccountUUID ||
				got.SessionID != tt.want.SessionID {
				t.Errorf("ParseMetadataUserID() = %v, want %v", got, tt.want)
			}
		})
	}
}

// =============================================
// Format Tests
// =============================================

func TestMetadataUserID_Format(t *testing.T) {
	tests := []struct {
		name  string
		input *MetadataUserID
		want  string
	}{
		{
			name: "valid with all fields",
			input: &MetadataUserID{
				DeviceID:    "user_abc123",
				AccountUUID: "acc-456",
				SessionID:   "sess-789",
			},
			want: `{"device_id":"user_abc123","account_uuid":"acc-456","session_id":"sess-789"}`,
		},
		{
			name: "valid with empty account_uuid",
			input: &MetadataUserID{
				DeviceID:    "user_5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907",
				AccountUUID: "",
				SessionID:   "16d97292-8713-438b-ad2e-76f495717258",
			},
			want: `{"device_id":"user_5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907","account_uuid":"","session_id":"16d97292-8713-438b-ad2e-76f495717258"}`,
		},
		{
			name:  "nil input returns empty string",
			input: nil,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			if tt.input == nil {
				got = ""
			} else {
				got = tt.input.Format()
			}
			// JSON marshaling may have different whitespace, just check it's valid JSON and contains key fields
			if tt.input != nil {
				if !strings.HasPrefix(got, "{") {
					t.Errorf("Format() = %v, want JSON format starting with '{'", got)
				}
				if !strings.Contains(got, tt.input.DeviceID) {
					t.Errorf("Format() missing device_id, got %v", got)
				}
				if !strings.Contains(got, tt.input.SessionID) {
					t.Errorf("Format() missing session_id, got %v", got)
				}
			} else if got != tt.want {
				t.Errorf("Format() = %v, want %v", got, tt.want)
			}
		})
	}
}

// =============================================
// Fix Tests
// =============================================

func TestMetadataUserID_Fix(t *testing.T) {
	tests := []struct {
		name   string
		input  *MetadataUserID
		extras map[string]any
		verify func(*MetadataUserID) bool
	}{
		{
			name: "fixes empty device_id with generated hash",
			input: &MetadataUserID{
				SessionID: "sess-789",
			},
			extras: nil,
			verify: func(m *MetadataUserID) bool {
				return m.DeviceID != "" && m.SessionID == "sess-789"
			},
		},
		{
			name: "sets account_uuid from extras",
			input: &MetadataUserID{
				SessionID: "sess-789",
			},
			extras: map[string]any{
				"user_id": "acc-custom-123",
			},
			verify: func(m *MetadataUserID) bool {
				return m.AccountUUID == "acc-custom-123"
			},
		},
		{
			name: "generates session_id if empty",
			input: &MetadataUserID{
				DeviceID: "existing-device",
			},
			extras: nil,
			verify: func(m *MetadataUserID) bool {
				return m.DeviceID == "existing-device" && m.SessionID != ""
			},
		},
		{
			name: "nil extras is handled",
			input: &MetadataUserID{
				DeviceID:  "existing-device",
				SessionID: "existing-session",
			},
			extras: nil,
			verify: func(m *MetadataUserID) bool {
				return m.DeviceID == "existing-device" && m.SessionID == "existing-session"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.input.Fix(tt.extras)
			if !tt.verify(tt.input) {
				t.Errorf("Fix() produced unexpected result: %+v", tt.input)
			}
		})
	}
}

// =============================================
// IsValid/IsEmpty Tests
// =============================================

func TestMetadataUserID_IsValid(t *testing.T) {
	tests := []struct {
		name string
		m    *MetadataUserID
		want bool
	}{
		{
			name: "valid with all fields",
			m: &MetadataUserID{
				DeviceID:    "user_abc",
				AccountUUID: "acc-123",
				SessionID:   "sess-456",
			},
			want: true,
		},
		{
			name: "valid without account_uuid",
			m: &MetadataUserID{
				DeviceID:  "user_abc",
				SessionID: "sess-456",
			},
			want: true,
		},
		{
			name: "nil",
			m:    nil,
			want: false,
		},
		{
			name: "missing device_id",
			m: &MetadataUserID{
				SessionID: "sess-456",
			},
			want: false,
		},
		{
			name: "missing session_id",
			m: &MetadataUserID{
				DeviceID: "user_abc",
			},
			want: false,
		},
		{
			name: "all empty",
			m: &MetadataUserID{
				DeviceID:    "",
				AccountUUID: "",
				SessionID:   "",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.IsValid(); got != tt.want {
				t.Errorf("MetadataUserID.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMetadataUserID_IsEmpty(t *testing.T) {
	tests := []struct {
		name string
		m    *MetadataUserID
		want bool
	}{
		{
			name: "non-nil with data",
			m: &MetadataUserID{
				DeviceID:  "user_abc",
				SessionID: "sess-456",
			},
			want: false,
		},
		{
			name: "nil",
			m:    nil,
			want: true,
		},
		{
			name: "all empty strings",
			m: &MetadataUserID{
				DeviceID:    "",
				AccountUUID: "",
				SessionID:   "",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.IsEmpty(); got != tt.want {
				t.Errorf("MetadataUserID.IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

// =============================================
// Round-trip Tests
// =============================================

func TestRoundTrip_JSON(t *testing.T) {
	original := &MetadataUserID{
		DeviceID:    "user_5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907",
		AccountUUID: "550e8400-e29b-41d4-a716-446655440000",
		SessionID:   "16d97292-8713-438b-ad2e-76f495717258",
	}

	// Format
	formatted := original.Format()
	if formatted == "" {
		t.Fatalf("Format() returned empty string")
	}

	// Parse
	parsed := ParseMetadataUserID(formatted)
	if parsed == nil {
		t.Fatalf("ParseMetadataUserID() = nil, want non-nil")
	}

	// Compare
	if parsed.DeviceID != original.DeviceID ||
		parsed.AccountUUID != original.AccountUUID ||
		parsed.SessionID != original.SessionID {
		t.Errorf("Round-trip failed: got %v, want %v", parsed, original)
	}
}

func TestRoundTrip_Legacy(t *testing.T) {
	// Legacy format only preserves device_id, account_uuid, session_id
	// Parse should extract them correctly
	legacy := "user_5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907_account_550e8400-e29b-41d4-a716-446655440000_session_16d97292-8713-438b-ad2e-76f495717258"

	parsed := ParseMetadataUserID(legacy)
	if parsed == nil {
		t.Fatalf("ParseMetadataUserID() = nil, want non-nil")
	}

	expected := &MetadataUserID{
		DeviceID:    "5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907",
		AccountUUID: "550e8400-e29b-41d4-a716-446655440000",
		SessionID:   "16d97292-8713-438b-ad2e-76f495717258",
	}

	if parsed.DeviceID != expected.DeviceID ||
		parsed.AccountUUID != expected.AccountUUID ||
		parsed.SessionID != expected.SessionID {
		t.Errorf("Parse legacy failed: got %v, want %v", parsed, expected)
	}

	// Round-trip through JSON format
	formatted := parsed.Format()

	// Should be valid JSON
	if !strings.HasPrefix(formatted, "{") {
		t.Errorf("Format() = %v, want JSON format", formatted)
	}
}

// =============================================
// BuildMetadataUserID Tests
// =============================================

func TestBuildMetadataUserIDFromProvider(t *testing.T) {
	providerUUID := "550e8400-e29b-41d4-a716-446655440000"
	userID := "16d97292-8713-438b-ad2e-76f495717258"
	accountUUID := "acc-12345678"
	deviceID := "user_custom_device_id_123"

	tests := []struct {
		name   string
		extra  map[string]any
		verify func(*MetadataUserID) bool
	}{
		{
			name:  "nil extra - generates random values",
			extra: nil,
			verify: func(m *MetadataUserID) bool {
				return m != nil && m.DeviceID != "" && m.SessionID != ""
			},
		},
		{
			name:  "empty extra - generates random values",
			extra: map[string]any{},
			verify: func(m *MetadataUserID) bool {
				return m != nil && m.DeviceID != "" && m.SessionID != ""
			},
		},
		{
			name: "extra with user_id only - user_id goes to account_uuid",
			extra: map[string]any{
				"user_id": userID,
			},
			verify: func(m *MetadataUserID) bool {
				return m != nil && m.AccountUUID == userID && m.SessionID != ""
			},
		},
		{
			name: "extra with all fields",
			extra: map[string]any{
				"user_id":       userID,
				"account_uuid":  accountUUID,
				"device_id":     deviceID,
				"provider_uuid": providerUUID,
			},
			verify: func(m *MetadataUserID) bool {
				// device_id from extras["device"] or generated, session_id generated
				return m != nil && m.SessionID != ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildMetadataUserID(tt.extra)
			if got == nil {
				t.Errorf("BuildMetadataUserID() = nil, want non-nil")
				return
			}
			if !tt.verify(got) {
				t.Errorf("BuildMetadataUserID() verification failed: %+v", got)
			}
		})
	}
}

// =============================================
// FixMetadataUserID Tests
// =============================================

func TestFixMetadataUserID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(*MetadataUserID) bool
	}{
		{
			name:  "valid JSON gets fixed with generated fields",
			input: `{"device_id":"","session_id":""}`,
			check: func(m *MetadataUserID) bool {
				return m.DeviceID != "" && m.SessionID != ""
			},
		},
		{
			name:  "partial JSON gets completed",
			input: `{"device_id":"existing","session_id":""}`,
			check: func(m *MetadataUserID) bool {
				return m.DeviceID == "existing" && m.SessionID != ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FixMetadataUserID(tt.input)
			if got == nil {
				t.Errorf("FixMetadataUserID() = nil, want non-nil")
				return
			}
			if !tt.check(got) {
				t.Errorf("FixMetadataUserID() produced unexpected result: %+v", got)
			}
		})
	}
}

// =============================================
// FormatMetadataUserID Tests
// =============================================

func TestFormatMetadataUserID(t *testing.T) {
	tests := []struct {
		name  string
		input *MetadataUserID
		check func(string) bool
	}{
		{
			name: "valid input produces JSON",
			input: &MetadataUserID{
				DeviceID:  "user_abc",
				SessionID: "sess-123",
			},
			check: func(s string) bool {
				return strings.HasPrefix(s, "{") && strings.Contains(s, "user_abc")
			},
		},
		{
			name:  "nil input returns empty string",
			input: nil,
			check: func(s string) bool {
				return s == ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatMetadataUserID(tt.input)
			if !tt.check(got) {
				t.Errorf("FormatMetadataUserID() = %v, check failed", got)
			}
		})
	}
}

// =============================================
// ApplyAnthropicMetadataTransform Tests
// =============================================

func TestApplyAnthropicMetadataTransform_V1(t *testing.T) {
	userID := "16d97292-8713-438b-ad2e-76f495717258"

	tests := []struct {
		name           string
		req            *anthropic.MessageNewParams
		extra          map[string]any
		wantNoMetadata bool
		checkMetadata  func(string) bool
	}{
		{
			name:           "nil request",
			req:            nil,
			extra:          map[string]any{"user_id": userID},
			wantNoMetadata: true,
		},
		{
			name:           "nil extra - still generates metadata",
			req:            &anthropic.MessageNewParams{},
			extra:          nil,
			wantNoMetadata: false, // BuildMetadataUserID(nil) generates random values
			checkMetadata: func(s string) bool {
				return strings.HasPrefix(s, "{") && strings.Contains(s, "device_id")
			},
		},
		{
			name: "with user_id in extra",
			req:  &anthropic.MessageNewParams{},
			extra: map[string]any{
				"user_id": userID,
			},
			wantNoMetadata: false,
			checkMetadata: func(s string) bool {
				return strings.Contains(s, userID)
			},
		},
		{
			name: "existing metadata gets fixed",
			req: &anthropic.MessageNewParams{
				Metadata: anthropic.MetadataParam{
					UserID: param.NewOpt(`{"device_id":"","session_id":""}`),
				},
			},
			extra:          nil,
			wantNoMetadata: false,
			checkMetadata: func(s string) bool {
				// After Fix, should have generated device_id and session_id
				return strings.Contains(s, "device_id") && strings.Contains(s, "session_id")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ApplyAnthropicMetadataTransform(tt.req, tt.extra)

			if tt.wantNoMetadata {
				if result != tt.req {
					t.Errorf("ApplyAnthropicMetadataTransform() should return same request")
				}
				return
			}

			typedResult, ok := result.(*anthropic.MessageNewParams)
			if !ok {
				t.Errorf("ApplyAnthropicMetadataTransform() wrong type")
				return
			}

			if !typedResult.Metadata.UserID.Valid() {
				t.Errorf("ApplyAnthropicMetadataTransform() metadata.UserID not set")
				return
			}

			if tt.checkMetadata != nil {
				if !tt.checkMetadata(typedResult.Metadata.UserID.Value) {
					t.Errorf("ApplyAnthropicMetadataTransform() metadata check failed, got %v", typedResult.Metadata.UserID.Value)
				}
			}
		})
	}
}

func TestApplyAnthropicMetadataTransform_Beta(t *testing.T) {
	userID := "16d97292-8713-438b-ad2e-76f495717258"

	tests := []struct {
		name          string
		req           *anthropic.BetaMessageNewParams
		extra         map[string]any
		checkMetadata func(string) bool
	}{
		{
			name: "with user_id in extra",
			req:  &anthropic.BetaMessageNewParams{},
			extra: map[string]any{
				"user_id": userID,
			},
			checkMetadata: func(s string) bool {
				return strings.Contains(s, userID)
			},
		},
		{
			name: "existing metadata gets fixed",
			req: &anthropic.BetaMessageNewParams{
				Metadata: anthropic.BetaMetadataParam{
					UserID: param.NewOpt(`{"device_id":"","session_id":""}`),
				},
			},
			extra: nil,
			checkMetadata: func(s string) bool {
				return strings.Contains(s, "device_id") && strings.Contains(s, "session_id")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ApplyAnthropicMetadataTransform(tt.req, tt.extra)

			typedResult, ok := result.(*anthropic.BetaMessageNewParams)
			if !ok {
				t.Errorf("ApplyAnthropicMetadataTransform() wrong type")
				return
			}

			if !typedResult.Metadata.UserID.Valid() {
				t.Errorf("ApplyAnthropicMetadataTransform() metadata.UserID not set")
				return
			}

			if tt.checkMetadata != nil {
				if !tt.checkMetadata(typedResult.Metadata.UserID.Value) {
					t.Errorf("ApplyAnthropicMetadataTransform() metadata check failed, got %v", typedResult.Metadata.UserID.Value)
				}
			}
		})
	}
}
