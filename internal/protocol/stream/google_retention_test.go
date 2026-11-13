package stream

import (
	"errors"
	"iter"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"google.golang.org/genai"
)

func TestGoogleConversionsRetainTailUsageAndToolIndices(t *testing.T) {
	for _, route := range []string{"chat", "anthropic", "anthropic_beta"} {
		for _, fail := range []bool{false, true} {
			t.Run(route+"/"+map[bool]string{false: "usage", true: "late_error"}[fail], func(t *testing.T) {
				wantErr := errors.New("tail failed")
				seq := iter.Seq2[*genai.GenerateContentResponse, error](func(yield func(*genai.GenerateContentResponse, error) bool) {
					if !yield(&genai.GenerateContentResponse{Candidates: []*genai.Candidate{{Content: &genai.Content{Parts: []*genai.Part{{Text: "hello"}, {FunctionCall: &genai.FunctionCall{ID: "t1", Name: "first", Args: map[string]any{"a": 1}}}, {FunctionCall: &genai.FunctionCall{ID: "t2", Name: "second", Args: map[string]any{}}}, {Text: "tail"}}}, FinishReason: genai.FinishReasonStop}}}, nil) {
						return
					}
					if fail {
						yield(nil, wantErr)
						return
					}
					yield(&genai.GenerateContentResponse{UsageMetadata: &genai.GenerateContentResponseUsageMetadata{PromptTokenCount: 190000, CandidatesTokenCount: 10, ThoughtsTokenCount: 3, CachedContentTokenCount: 5000, TotalTokenCount: 190013}}, nil)
				})
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
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
				body := recorder.Body.String()
				if fail {
					require.ErrorIs(t, err, wantErr)
					require.Contains(t, body, "tail failed")
					require.NotContains(t, body, "[DONE]")
					require.NotContains(t, body, `"type":"message_stop"`)
					return
				}
				require.NoError(t, err)
				if route == "chat" {
					require.Contains(t, body, `"prompt_tokens":190000`)
					require.Contains(t, body, `"completion_tokens":13`)
					require.Contains(t, body, `"cached_tokens":5000`)
					require.Contains(t, body, `"reasoning_tokens":3`)
				} else {
					require.Contains(t, body, `"input_tokens":185000`)
					require.Contains(t, body, `"output_tokens":13`)
					require.Contains(t, body, `"cache_read_input_tokens":5000`)
				}
				var starts, stops []int64
				for _, line := range strings.Split(body, "\n") {
					if !strings.HasPrefix(line, "data:") {
						continue
					}
					data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
					if route == "chat" {
						for _, tc := range gjson.Get(data, "choices.0.delta.tool_calls").Array() {
							starts = append(starts, tc.Get("index").Int())
						}
					} else {
						switch gjson.Get(data, "type").String() {
						case "content_block_start":
							starts = append(starts, gjson.Get(data, "index").Int())
						case "content_block_stop":
							stops = append(stops, gjson.Get(data, "index").Int())
						}
					}
				}
				if route == "chat" {
					require.Equal(t, []int64{0, 1}, starts)
				} else {
					require.Equal(t, []int64{0, 1, 2, 3}, starts)
					require.Equal(t, starts, stops)
					require.Contains(t, body, `"stop_reason":"tool_use"`)
				}
			})
		}
	}
}
