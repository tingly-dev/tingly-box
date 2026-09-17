package protocolserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func overlayCtx(t *testing.T, header string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/tingly/claude_code/v1/messages", nil)
	if header != "" {
		c.Request.Header.Set(typ.ProbeFlagsHeader, header)
	}
	return c
}

func TestApplyProbeFlagOverlay(t *testing.T) {
	base := typ.RuleFlags{SkipUsage: true, ClaudeCodeCompat: true}

	t.Run("no header is identity", func(t *testing.T) {
		assert.Equal(t, base, applyProbeFlagOverlay(overlayCtx(t, ""), base))
	})

	t.Run("overlay overrides and clears", func(t *testing.T) {
		enc, err := typ.EncodeFlagOverlay(typ.FlagOverlay{
			"skip_usage":                json.RawMessage("false"),
			"use_max_completion_tokens": json.RawMessage("true"),
		})
		require.NoError(t, err)
		out := applyProbeFlagOverlay(overlayCtx(t, enc), base)
		assert.False(t, out.SkipUsage)
		assert.True(t, out.UseMaxCompletionTokens)
		assert.True(t, out.ClaudeCodeCompat, "untouched")
	})

	t.Run("malformed header is ignored", func(t *testing.T) {
		assert.Equal(t, base, applyProbeFlagOverlay(overlayCtx(t, "@@@"), base))
	})

	t.Run("nil context is safe", func(t *testing.T) {
		assert.Equal(t, base, applyProbeFlagOverlay(nil, base))
	})
}
