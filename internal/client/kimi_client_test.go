package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestStripKimiPrefix(t *testing.T) {
	client := &KimiClient{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "lowercase prefix",
			input:    "kimi-k2",
			expected: "k2",
		},
		{
			name:     "uppercase prefix - case insensitive match",
			input:    "Kimi-K2",
			expected: "K2",
		},
		{
			name:     "mixed case prefix",
			input:    "KiMi-k2",
			expected: "k2",
		},
		{
			name:     "no prefix",
			input:    "k2",
			expected: "k2",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace trimmed",
			input:    "   ",
			expected: "",
		},
		{
			name:     "kimi-gpt-4",
			input:    "kimi-gpt-4",
			expected: "gpt-4",
		},
		{
			name:     "KIMI- prefix uppercase",
			input:    "KIMI-k2",
			expected: "k2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.stripKimiPrefix(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetDeviceID(t *testing.T) {
	tests := []struct {
		name     string
		provider *typ.Provider
		expected string
	}{
		{
			name:     "nil provider",
			provider: nil,
			expected: "",
		},
		{
			name:     "nil OAuth detail",
			provider: &typ.Provider{},
			expected: "",
		},
		{
			name: "valid device ID",
			provider: &typ.Provider{
				OAuthDetail: &typ.OAuthDetail{
					DeviceID: "test-device-123",
				},
			},
			expected: "test-device-123",
		},
		{
			name: "empty device ID",
			provider: &typ.Provider{
				OAuthDetail: &typ.OAuthDetail{
					DeviceID: "",
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getDeviceID(tt.provider)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestKimiRoundTripperConstruction(t *testing.T) {
	tests := []struct {
		name     string
		provider *typ.Provider
		expectID bool
	}{
		{
			name:     "nil provider",
			provider: nil,
			expectID: false,
		},
		{
			name:     "nil OAuth detail",
			provider: &typ.Provider{},
			expectID: false,
		},
		{
			name: "provider with device ID",
			provider: &typ.Provider{
				OAuthDetail: &typ.OAuthDetail{
					DeviceID: "test-device",
				},
			},
			expectID: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newKimiRoundTripper(nil, tt.provider)
			assert.NotNil(t, rt)

			if tt.expectID {
				assert.Equal(t, tt.provider.OAuthDetail.DeviceID, rt.deviceID)
			} else {
				assert.Equal(t, "", rt.deviceID)
			}

			// Verify static fields are set
			assert.NotEmpty(t, rt.deviceName, "deviceName should be set")
			assert.NotEmpty(t, rt.deviceModel, "deviceModel should be set")
			assert.NotEmpty(t, rt.osVersion, "osVersion should be set")
		})
	}
}
