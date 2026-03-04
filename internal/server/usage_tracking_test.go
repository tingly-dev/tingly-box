package server

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestSanitizeErrorCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{"nil error", nil, ""},
		{"simple error", errors.New("test error"), "test error"},
		{"context canceled", context.Canceled, "context canceled"},
		{"context deadline exceeded", context.DeadlineExceeded, "context deadline exceeded"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeErrorCode(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUpdateServiceStats(t *testing.T) {
	s := &Server{}
	// Minimal test - just verify it doesn't panic with nil inputs
	s.updateServiceStats(nil, nil, "", 0, 0, 0)

	// Test with actual service but no config - should handle gracefully
	rule := &typ.Rule{
		UUID: "test-rule",
		Services: []*loadbalance.Service{
			{
				Active:   true,
				Provider: "test-provider",
				Model:    "gpt-4",
			},
		},
	}
	provider := &typ.Provider{UUID: "test-provider", Name: "test"}

	// Should not panic even without config set
	s.updateServiceStats(rule, provider, "gpt-4", 100, 50, 150)
}

func TestTrackUsageFromContext_DoesNotPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name   string
		setup  func(*gin.Context)
		input  int
		output int
		err    error
	}{
		{
			name: "with full context",
			setup: func(c *gin.Context) {
				rule := &typ.Rule{UUID: "test-rule"}
				provider := &typ.Provider{UUID: "test-provider", Name: "test"}
				SetTrackingContext(c, rule, provider, "gpt-4", "gpt-4", false)
			},
			input:  100,
			output: 50,
			err:    nil,
		},
		{
			name: "with error",
			setup: func(c *gin.Context) {
				rule := &typ.Rule{UUID: "test-rule"}
				provider := &typ.Provider{UUID: "test-provider", Name: "test"}
				SetTrackingContext(c, rule, provider, "gpt-4", "gpt-4", false)
			},
			input:  0,
			output: 0,
			err:    errors.New("test error"),
		},
		{
			name:   "without context",
			setup:  func(c *gin.Context) {},
			input:  100,
			output: 50,
			err:    nil,
		},
		{
			name: "with context canceled",
			setup: func(c *gin.Context) {
				rule := &typ.Rule{UUID: "test-rule"}
				provider := &typ.Provider{UUID: "test-provider", Name: "test"}
				SetTrackingContext(c, rule, provider, "gpt-4", "gpt-4", false)
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				c.Request = &http.Request{}
				c.Request = c.Request.WithContext(ctx)
			},
			input:  100,
			output: 50,
			err:    context.Canceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(nil)
			tt.setup(c)

			s := &Server{}

			// Should not panic
			s.trackUsageFromContext(c, tt.input, tt.output, tt.err)
		})
	}
}
