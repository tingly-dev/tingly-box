package ops

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

// =============================================
// Anthropic metadata-transform tests (the metadata.user_id model itself is
// tested in internal/protocol/metaid).
// =============================================

func TestApplyAnthropicV1MetadataTransform(t *testing.T) {
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
			name:           "nil extra - no metadata generated",
			req:            &anthropic.MessageNewParams{},
			extra:          nil,
			wantNoMetadata: true, // metaid.BuildMetadataUserID(nil) returns nil due to missing required fields
		},
		{
			name: "with user_id in extra - also needs device",
			req:  &anthropic.MessageNewParams{},
			extra: map[string]any{
				"user_id": userID,
				"device":  "test-device",
			},
			wantNoMetadata: false,
			checkMetadata: func(s string) bool {
				return strings.Contains(s, userID)
			},
		},
		{
			name: "existing metadata with required fields gets fixed",
			req: &anthropic.MessageNewParams{
				Metadata: anthropic.MetadataParam{
					UserID: param.NewOpt(`{"device_id":"existing-device","account_uuid":"test-account","session_id":""}`),
				},
			},
			extra:          nil,
			wantNoMetadata: false,
			checkMetadata: func(s string) bool {
				// After Fix, should have generated session_id
				return strings.Contains(s, "device_id") && strings.Contains(s, "session_id")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ApplyAnthropicV1MetadataTransform(tt.req, tt.extra)

			if tt.wantNoMetadata {
				if result != tt.req {
					t.Errorf("ApplyAnthropicV1MetadataTransform() should return same request")
				}
				return
			}

			if !result.Metadata.UserID.Valid() {
				t.Errorf("ApplyAnthropicV1MetadataTransform() metadata.UserID not set")
				return
			}

			if tt.checkMetadata != nil {
				if !tt.checkMetadata(result.Metadata.UserID.Value) {
					t.Errorf("ApplyAnthropicV1MetadataTransform() metadata check failed, got %v", result.Metadata.UserID.Value)
				}
			}
		})
	}
}

func TestApplyAnthropicBetaMetadataTransform(t *testing.T) {
	userID := "16d97292-8713-438b-ad2e-76f495717258"

	tests := []struct {
		name          string
		req           *anthropic.BetaMessageNewParams
		extra         map[string]any
		checkMetadata func(string) bool
	}{
		{
			name: "with user_id in extra - also needs device",
			req:  &anthropic.BetaMessageNewParams{},
			extra: map[string]any{
				"user_id": userID,
				"device":  "test-device",
			},
			checkMetadata: func(s string) bool {
				return strings.Contains(s, userID)
			},
		},
		{
			name: "existing metadata with required fields gets fixed",
			req: &anthropic.BetaMessageNewParams{
				Metadata: anthropic.BetaMetadataParam{
					UserID: param.NewOpt(`{"device_id":"existing-device","account_uuid":"test-account","session_id":""}`),
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
			result := ApplyAnthropicBetaMetadataTransform(tt.req, tt.extra)

			if !result.Metadata.UserID.Valid() {
				t.Errorf("ApplyAnthropicBetaMetadataTransform() metadata.UserID not set")
				return
			}

			if tt.checkMetadata != nil {
				if !tt.checkMetadata(result.Metadata.UserID.Value) {
					t.Errorf("ApplyAnthropicBetaMetadataTransform() metadata check failed, got %v", result.Metadata.UserID.Value)
				}
			}
		})
	}
}
