package server

import (
	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// trackUsage records token usage using the UsageTracker.
// It will also record to OTel if the token tracker is available in the gin context.
func (s *Server) trackUsage(c *gin.Context, rule *typ.Rule, provider *typ.Provider, model, requestModel string, inputTokens, outputTokens int, streamed bool, status, errorCode string) {
	// Set token tracker in context for RecordUsage to use
	if s.tokenTracker != nil {
		c.Set("token_tracker", s.tokenTracker)
	}

	tracker := s.NewUsageTracker()
	tracker.RecordUsage(c, rule, provider, model, requestModel, inputTokens, outputTokens, streamed, status, errorCode)
}
