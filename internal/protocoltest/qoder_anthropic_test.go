package protocoltest

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/oauth"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestQoderAnthropic verifies the real gateway bridge and the Qoder client
// together. The Qoder wire server is synthetic: no real credential is used.
func TestQoderAnthropic(t *testing.T) {
	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/algo/api/v2/service/pro/sse/agent_chat_generation" {
			t.Errorf("unexpected upstream path %s", r.URL.Path)
			http.Error(w, "wrong path", 400)
			return
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		decoded, err := decodeQoderFixture(raw)
		if err != nil {
			t.Error(err)
			return
		}
		var request struct {
			Messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"messages"`
			Stream bool `json:"stream"`
		}
		if err := json.Unmarshal(decoded, &request); err != nil {
			t.Error(err)
			return
		}
		if !request.Stream || len(request.Messages) == 0 {
			t.Error("missing upstream messages or streaming")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, chunk := range []string{
			`{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"auto","choices":[{"index":0,"delta":{"role":"assistant","content":"Paris"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"auto","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
		} {
			env, _ := json.Marshal(map[string]any{"statusCodeValue": 200, "body": chunk})
			fmt.Fprintf(w, "data: %s\n\n", env)
		}
		fmt.Fprint(w, "event:finish\ndata: {}\n\n")
	}))
	defer upstream.Close()
	env := NewTestEnv(t)
	defer env.Close()
	for _, source := range []protocol.APIType{protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta} {
		for _, issuer := range []ai.Issuer{ai.IssuerQoder, ai.IssuerQoderCN} {
			for _, streaming := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", source, issuer, streaming), func(t *testing.T) {
					s := TextScenario()
					s.Name = fmt.Sprintf("qoder-%s-%t", issuer, streaming)
					env.setupRouteCore(source, protocol.TypeOpenAIChat, s, nil, func(p *typ.Provider) {
						p.APIBase = upstream.URL
						p.AuthType = typ.AuthTypeOAuth
						p.Token = ""
						p.OAuthDetail = &typ.OAuthDetail{Issuer: issuer, AccessToken: "synthetic", ExtraFields: map[string]any{"account_id": "test-user", oauth.QoderMachineID: "test-machine"}}
					})
					before := hits.Load()
					result := env.SendAs(t, source, protocol.TypeOpenAIChat, s, streaming)
					require.Equal(t, 200, result.HTTPStatus)
					require.Contains(t, result.Content, "Paris")
					require.Greater(t, hits.Load(), before)
				})
			}
		}
	}
}

func decodeQoderFixture(encoded []byte) ([]byte, error) {
	s := string(encoded)
	n := len(s) / 3
	s = s[len(s)-n:] + s[n:len(s)-n] + s[:n]
	const custom = "_doRTgHZBKcGVjlvpC,@aFSx#DPuNJme&i*MzLOEn)sUrthbf%Y^w.(kIQyXqWA!"
	const standard = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	b := []byte(s)
	for i, c := range b {
		if c == '$' {
			b[i] = '='
			continue
		}
		k := strings.IndexByte(custom, c)
		if k < 0 {
			return nil, fmt.Errorf("invalid encoding")
		}
		b[i] = standard[k]
	}
	return base64.StdEncoding.DecodeString(string(b))
}

func TestQoderAnthropicTools(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encoded, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		body, err := decodeQoderFixture(encoded)
		if err != nil {
			t.Error(err)
			return
		}
		for _, expected := range []string{`"get_weather"`, `"tool_call_id":"call_1"`, `"role":"tool"`, `Paris`} {
			if !strings.Contains(string(body), expected) {
				t.Errorf("upstream request missing %s: %s", expected, body)
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, chunk := range []string{
			`{"id":"chatcmpl-tool","object":"chat.completion.chunk","created":1,"model":"auto","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_2","type":"function","function":{"name":"get_weather","arguments":"{\"location\":\"London\"}"}}]},"finish_reason":null}]}`,
			`{"id":"chatcmpl-tool","object":"chat.completion.chunk","created":1,"model":"auto","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
		} {
			e, _ := json.Marshal(map[string]any{"statusCodeValue": 200, "body": chunk})
			fmt.Fprintf(w, "data: %s\n\n", e)
		}
		fmt.Fprint(w, "event:finish\ndata: {}\n\n")
	}))
	defer upstream.Close()
	env := NewTestEnv(t)
	defer env.Close()
	for _, source := range []protocol.APIType{protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta} {
		s := ToolUseScenario()
		s.Name = "qoder-tools"
		env.setupRouteCore(source, protocol.TypeOpenAIChat, s, nil, func(p *typ.Provider) {
			p.APIBase = upstream.URL
			p.AuthType = typ.AuthTypeOAuth
			p.Token = ""
			p.OAuthDetail = &typ.OAuthDetail{Issuer: ai.IssuerQoder, AccessToken: "synthetic", ExtraFields: map[string]any{"account_id": "test-user", oauth.QoderMachineID: "test-machine"}}
		})
		model := env.findRouteModel(source, protocol.TypeOpenAIChat, s.Name)
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", source, streaming), func(t *testing.T) {
				path, _ := buildRequest(source, model, streaming)
				request := fmt.Sprintf(`{"model":%q,"max_tokens":1024,"stream":%t,"tools":[{"name":"get_weather","input_schema":{"type":"object","properties":{"location":{"type":"string"}}}}],"messages":[{"role":"user","content":"Paris?"},{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"get_weather","input":{"location":"Paris"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"18C"}]}]}`, model, streaming)
				status, body := sendRaw(t, env, path, []byte(request))
				require.Equal(t, 200, status, body)
				require.Contains(t, body, "get_weather")
				require.Contains(t, body, "tool_use")
				require.Contains(t, body, "London")
			})
		}
	}
}
