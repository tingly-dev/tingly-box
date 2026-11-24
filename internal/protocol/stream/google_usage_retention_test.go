package stream

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
	"net/http/httptest"
	"testing"
)

func TestGoogleStreamingUsageDetails(t *testing.T) {
	for _, route := range []string{"chat", "anthropic", "anthropic_beta"} {
		t.Run(route, func(t *testing.T) {
			seq := func(yield func(*genai.GenerateContentResponse, error) bool) {
				yield(&genai.GenerateContentResponse{Candidates: []*genai.Candidate{{FinishReason: genai.FinishReasonStop}}, UsageMetadata: &genai.GenerateContentResponseUsageMetadata{PromptTokenCount: 100, CachedContentTokenCount: 40, CandidatesTokenCount: 10, ThoughtsTokenCount: 5, TotalTokenCount: 115}}, nil)
			}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/", nil)
			var err error
			switch route {
			case "chat":
				err = HandleGoogleToOpenAIStreamResponse(c, seq, "model")
			case "anthropic":
				_, err = HandleGoogleToAnthropicStreamResponse(c, seq, "model")
			default:
				_, err = HandleGoogleToAnthropicBetaStreamResponse(c, seq, "model")
			}
			require.NoError(t, err)
			body := w.Body.String()
			if route == "chat" {
				require.Contains(t, body, `"prompt_tokens":100`)
				require.Contains(t, body, `"completion_tokens":15`)
				require.Contains(t, body, `"cached_tokens":40`)
				require.Contains(t, body, `"reasoning_tokens":5`)
			} else {
				require.Contains(t, body, `"input_tokens":60`)
				require.Contains(t, body, `"output_tokens":15`)
				require.Contains(t, body, `"cache_read_input_tokens":40`)
			}
		})
	}
}
