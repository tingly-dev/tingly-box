package stream

import (
	"iter"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"google.golang.org/genai"
)

func TestGoogleStreamUsageNormalizesInclusiveCache(t *testing.T) {
	for name, handler := range map[string]func(*gin.Context, iter.Seq2[*genai.GenerateContentResponse, error], string) (*protocol.TokenUsage, error){
		"anthropic":      HandleGoogleToAnthropicStreamResponse,
		"anthropic_beta": HandleGoogleToAnthropicBetaStreamResponse,
	} {
		for _, stop := range []bool{false, true} {
			t.Run(name+map[bool]string{false: "/eof", true: "/stop"}[stop], func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/messages", nil)
				candidate := &genai.Candidate{Content: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: "ok"}}}}
				if stop {
					candidate.FinishReason = genai.FinishReasonStop
				}
				response := &genai.GenerateContentResponse{Candidates: []*genai.Candidate{candidate}, UsageMetadata: &genai.GenerateContentResponseUsageMetadata{PromptTokenCount: 12, CachedContentTokenCount: 4, CandidatesTokenCount: 8}}
				usage, err := handler(c, func(yield func(*genai.GenerateContentResponse, error) bool) { yield(response, nil) }, "model")
				require.NoError(t, err)
				require.Equal(t, 8, usage.InputTokens)
				require.Equal(t, 20, usage.PromptTotalTokens()+usage.OutputTokens)
			})
		}
	}
}
