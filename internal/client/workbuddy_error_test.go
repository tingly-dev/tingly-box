package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestWorkBuddyErrorSurvivesSDK(t *testing.T) {
	for _, tc := range []struct {
		name, body, kind, code, message string
		status                          int
	}{
		{"channel", `{"code":11128,"msg":"Illegal API invocation from an unapproved channel"}`, "workbuddy_channel_refused", "11128", "unapproved channel", 400},
		{"client", `{"code":10085,"msg":"request illegal"}`, "workbuddy_channel_refused", "10085", "allowed client channel", 400},
		{"restriction", `{"code":11140,"msg":"request illegal","displayMsg":{"en":"Your account has been restricted due to unusual activity","zh":"账号存在异常行为，已被限制使用"}}`, "workbuddy_account_restricted", "11140", "submit feedback", 403},
		{"system shape", `{"code":11128,"msg":"first message is not system prompt"}`, "workbuddy_error", "11128", "first message", 400},
		{"schema", `{"code":11133,"msg":"Invalid request parameters"}`, "workbuddy_error", "11133", "Invalid request parameters", 400},
		{"auth", `{"code":401,"msg":"token expired"}`, "workbuddy_error", "401", "token expired", 401},
		{"rate limit", `{"code":6004,"msg":"too many requests"}`, "workbuddy_error", "6004", "too many requests", 429},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			p := &typ.Provider{UUID: "wb-errors", APIBase: server.URL + "/v2", APIStyle: protocol.APIStyleOpenAI, AuthType: typ.AuthTypeOAuth, OAuthDetail: &typ.OAuthDetail{Issuer: ai.IssuerWorkBuddyCN, AccessToken: "test"}}
			c, err := NewWorkBuddyClient(p, "m", typ.SessionID{})
			require.NoError(t, err)
			defer c.Close()
			params := openai.ChatCompletionNewParams{Model: "m", Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")}}
			// The same transport serves both SDK entry points.
			for _, streaming := range []bool{false, true} {
				if streaming {
					s := c.ChatCompletionsNewStreaming(context.Background(), params)
					for s.Next() {
					}
					err = s.Err()
					s.Close()
				} else {
					_, err = c.ChatCompletionsNew(context.Background(), params)
				}
				require.Error(t, err)
				f := protocol.ClassifyUpstreamFailure(fmt.Errorf("wrapped: %w", err), 500)
				require.Equal(t, tc.status, f.Status)
				require.Contains(t, f.Message, tc.message)
				var sdkErr *openai.Error
				require.ErrorAs(t, err, &sdkErr)
				require.Equal(t, tc.code, sdkErr.Code)
				require.Equal(t, tc.kind, sdkErr.Type)
				require.Equal(t, tc.kind == "workbuddy_channel_refused", IsWorkBuddyChannelRefusal(fmt.Errorf("wrapped: %w", err)))
				require.Contains(t, sdkErr.RawJSON(), `"msg"`)
			}
		})
	}
}
