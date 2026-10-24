package protocolserver

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	usageutil "github.com/tingly-dev/tingly-box/internal/protocol/usage"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestSharingQuotaCountsCacheWithoutDoubleCountingReasoning(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/tingly/team/v1/responses", nil)
	var charged int64
	c.Set(constant.CtxKeyChargeTokens, func(n int64) error { charged += n; return nil })
	ph := &ProtocolHandler{}
	provider := &typ.Provider{UUID: "test"}
	usage := &protocol.TokenUsage{InputTokens: 100, CacheReadTokens: 500, CacheWriteTokens: 20, OutputTokens: 50, ReasoningTokens: 30}
	for _, streamed := range []bool{false, true} {
		SetTrackingContext(c, &typ.Rule{UUID: "rule"}, provider, "model", "model", streamed)
		ph.trackUsageWithTokenUsage(c, usage, nil)
	}
	require.Equal(t, int64(1300), charged)
	ph.trackUsageFromContext(c, 10, 5, context.Canceled)
	require.Equal(t, int64(1315), charged)
}

func TestSharingQuotaWireInputTotals(t *testing.T) {
	openAI := openai.CompletionUsage{PromptTokens: 12, CompletionTokens: 8}
	openAI.PromptTokensDetails.CachedTokens = 4
	for name, usage := range map[string]*protocol.TokenUsage{
		"openai_inclusive_input":   usageutil.FromOpenAIChatCompletion(openAI),
		"anthropic_separate_cache": usageutil.FromAnthropicMessage(anthropic.Usage{InputTokens: 6, CacheCreationInputTokens: 2, CacheReadInputTokens: 4, OutputTokens: 8}),
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/tingly/team/v1/chat/completions", nil)
			var charged int64
			c.Set(constant.CtxKeyChargeTokens, func(n int64) error { charged += n; return nil })
			SetTrackingContext(c, &typ.Rule{UUID: "rule"}, &typ.Provider{UUID: "test"}, "model", "model", false)
			(&ProtocolHandler{}).trackUsageWithTokenUsage(c, usage, nil)
			require.Equal(t, int64(20), charged)
		})
	}
}

func TestToolEngineQuotaUsesInclusiveInput(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/tingly/team/v1/chat/completions", nil)
	var charged int64
	c.Set(constant.CtxKeyChargeTokens, func(n int64) error { charged += n; return nil })
	SetTrackingContext(c, &typ.Rule{UUID: "rule"}, &typ.Provider{UUID: "test"}, "model", "model", true)
	newServerOpsAdapter(&ProtocolHandler{}, nil).TrackUsage(c, 12, 8, 4)
	require.Equal(t, int64(20), charged)
}
