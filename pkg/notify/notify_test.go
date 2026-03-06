package notify

import (
	"errors"
	"testing"
	"time"
)

// TestNotificationValidate tests notification validation
func TestNotificationValidate(t *testing.T) {
	tests := []struct {
		name          string
		notification  *Notification
		wantErr       bool
		errContains   string
	}{
		{
			name: "valid notification",
			notification: &Notification{
				Message: "test message",
				Level:   LevelInfo,
			},
			wantErr: false,
		},
		{
			name: "missing message",
			notification: &Notification{
				Title: "test",
			},
			wantErr:     true,
			errContains: "message is required",
		},
		{
			name: "empty level defaults to info",
			notification: &Notification{
				Message: "test",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.notification.Validate()
			if tt.wantErr && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.errContains != "" && err != nil && !contains(err.Error(), tt.errContains) {
				t.Errorf("error should contain %q, got %q", tt.errContains, err.Error())
			}
		})
	}
}

// TestNotificationValidateTimestamp tests that timestamp is set automatically
func TestNotificationValidateTimestamp(t *testing.T) {
	n := &Notification{Message: "test"}
	before := time.Now()
	err := n.Validate()
	after := time.Now()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n.Timestamp.IsZero() {
		t.Error("timestamp should be set")
	}
	if n.Timestamp.Before(before) || n.Timestamp.After(after) {
		t.Error("timestamp should be between before and after")
	}
}

// TestNotificationLevel tests level functionality
func TestNotificationLevel(t *testing.T) {
	tests := []struct {
		level Level
		str   string
	}{
		{LevelDebug, "debug"},
		{LevelInfo, "info"},
		{LevelWarning, "warning"},
		{LevelError, "error"},
		{LevelCritical, "critical"},
	}

	for _, tt := range tests {
		t.Run(tt.str, func(t *testing.T) {
			if tt.level.String() != tt.str {
				t.Errorf("expected %q, got %q", tt.str, tt.level.String())
			}
		})
	}
}

// TestIsLevelAtLeast tests level comparison
func TestIsLevelAtLeast(t *testing.T) {
	tests := []struct {
		name   string
		level  Level
		min    Level
		expect bool
	}{
		{"debug >= info", LevelDebug, LevelInfo, false},
		{"info >= info", LevelInfo, LevelInfo, true},
		{"warning >= info", LevelWarning, LevelInfo, true},
		{"error >= warning", LevelError, LevelWarning, true},
		{"critical >= error", LevelCritical, LevelError, true},
		{"info >= debug", LevelInfo, LevelDebug, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsLevelAtLeast(tt.level, tt.min); got != tt.expect {
				t.Errorf("IsLevelAtLeast(%v, %v) = %v, want %v", tt.level, tt.min, got, tt.expect)
			}
		})
	}
}

// TestFormatError tests error formatting
func TestFormatError(t *testing.T) {
	baseErr := errors.New("base error")
	formatted := FormatError("webhook", baseErr)

	expected := "webhook: base error"
	if formatted.Error() != expected {
		t.Errorf("expected %q, got %q", expected, formatted.Error())
	}

	// Check that it wraps the original error
	if !errors.Is(formatted, baseErr) {
		t.Error("formatted error should wrap original error")
	}
}

// TestResult tests Result structure
func TestResult(t *testing.T) {
	r := &Result{
		Provider:  "test",
		Success:   true,
		MessageID: "msg-123",
		Latency:   100 * time.Millisecond,
	}

	if r.Provider != "test" {
		t.Errorf("unexpected provider: %v", r.Provider)
	}
	if !r.Success {
		t.Error("expected success to be true")
	}
	if r.MessageID != "msg-123" {
		t.Errorf("unexpected message ID: %v", r.MessageID)
	}
}

// TestLink tests Link structure
func TestLink(t *testing.T) {
	link := Link{
		Text: "Click me",
		URL:  "https://example.com",
	}

	if link.Text != "Click me" {
		t.Errorf("unexpected text: %v", link.Text)
	}
	if link.URL != "https://example.com" {
		t.Errorf("unexpected URL: %v", link.URL)
	}
}

// TestNotificationWithLinks tests notification with links
func TestNotificationWithLinks(t *testing.T) {
	n := &Notification{
		Message: "test",
		Links: []Link{
			{Text: "Link1", URL: "https://example.com/1"},
			{Text: "Link2", URL: "https://example.com/2"},
		},
		Tags: []string{"tag1", "tag2"},
		Metadata: map[string]interface{}{
			"key1": "value1",
			"key2": 123,
		},
	}

	if len(n.Links) != 2 {
		t.Errorf("expected 2 links, got %d", len(n.Links))
	}
	if len(n.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(n.Tags))
	}
	if len(n.Metadata) != 2 {
		t.Errorf("expected 2 metadata entries, got %d", len(n.Metadata))
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
