package weixin

import "testing"

// TestNewQRClient_BaseURL tests base URL handling.
func TestNewQRClient_BaseURL(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		expected string
	}{
		{"Default URL", "", defaultQRBaseURL},
		{"Explicit default", "https://ilinkai.weixin.qq.com", "https://ilinkai.weixin.qq.com"},
		{"Custom URL", "https://custom.wechat.com", "https://custom.wechat.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewQRClient(tt.baseURL)
			if client.baseURL != tt.expected {
				t.Errorf("Expected base URL '%s', got '%s'", tt.expected, client.baseURL)
			}
			if client.httpClient == nil {
				t.Error("Expected non-nil HTTP client")
			}
		})
	}
}
