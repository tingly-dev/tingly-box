package protocoltest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Exercise the real HTTP handler, transforms and SDK egress. The SDK upgrade
// must not turn formerly ignored input into upstream failures or erase history.
func TestAnthropicIngressWireCompatibility(t *testing.T) {
	for _, source := range []protocol.APIType{protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", source, streaming), func(t *testing.T) {
				env := NewTestEnv(t)
				s := TextScenario()
				env.SetupRoute(source, source, s)
				model := env.findRouteModel(source, source, s.Name)
				path, _ := buildRequest(source, model, streaming)
				body := mustMarshal(map[string]any{
					"model": model, "max_tokens": 64, "stream": streaming,
					"messages":    []map[string]any{{"role": "user", "content": "hi"}},
					"temperature": 0.0, "top_p": 0.9, "top_k": 40,
					"user":               "u-123",
					"context_management": map[string]any{"edits": []map[string]any{{"type": "clear_tool_uses_20250919"}}},
				})
				status, response := sendRawWithHeaders(t, env, path, body, map[string]string{
					"anthropic-beta": "context-management-2025-06-27",
				})
				require.Equal(t, http.StatusOK, status, response)
				wire := env.virtual.LastRequest(EndpointAnthropic).JSON()
				require.NotEmpty(t, wire)
				require.NotContains(t, wire, "user")
				require.Equal(t, float64(0), wire["temperature"])
				require.Equal(t, 0.9, wire["top_p"])
				require.Equal(t, float64(40), wire["top_k"])
				if source == protocol.TypeAnthropicBeta {
					require.Contains(t, wire, "context_management")
				} else {
					require.NotContains(t, wire, "context_management")
				}
			})
		}
	}
}

func TestAnthropicIngressCountTokensWire(t *testing.T) {
	for _, beta := range []bool{false, true} {
		t.Run(fmt.Sprintf("beta=%v", beta), func(t *testing.T) {
			env := NewTestEnv(t)
			captured := make(chan map[string]any, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				captured <- body
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"input_tokens":3}`))
			}))
			t.Cleanup(upstream.Close)
			s := TextScenario()
			source := protocol.TypeAnthropicV1
			if beta {
				source = protocol.TypeAnthropicBeta
			}
			env.setupRouteCore(source, source, s, nil, func(p *typ.Provider) { p.APIBase = upstream.URL })
			model := env.findRouteModel(source, source, s.Name)
			path, _ := buildRequest(source, model, false)
			path = strings.Replace(path, "/messages", "/messages/count_tokens", 1)
			body := mustMarshal(map[string]any{
				"model": model, "messages": []map[string]any{{"role": "user", "content": "hi"}},
				"max_tokens": 64, "stream": false, "temperature": 0.2, "top_p": 0.9, "top_k": 40, "user": "u-123",
			})
			status, response := sendRaw(t, env, path, body)
			require.Equal(t, http.StatusOK, status, response)
			select {
			case wire := <-captured:
				for _, key := range []string{"max_tokens", "stream", "temperature", "top_p", "top_k", "user"} {
					require.NotContains(t, wire, key)
				}
				require.Len(t, wire["messages"], 1)
			default:
				t.Fatal("count_tokens did not reach the provider")
			}
		})
	}
}

func TestAnthropicIngressUnknownRoleHTTP(t *testing.T) {
	for _, path := range []string{
		"/tingly/claude_code/v1/messages", "/tingly/claude_code/v1/messages?beta=true",
		"/tingly/claude_code/v1/messages/count_tokens", "/tingly/claude_code/v1/messages/count_tokens?beta=true",
	} {
		t.Run(path, func(t *testing.T) {
			env := NewTestEnv(t)
			status, response := sendRaw(t, env, path, []byte(`{"model":"no-route-needed","max_tokens":64,"messages":[
				{"role":"developer","content":"be brief"},{"role":"user","content":"hi"}]}`))
			require.Equal(t, http.StatusBadRequest, status, response)
			require.Contains(t, response, "messages[0].role")
			require.Contains(t, response, "developer")
			require.Contains(t, response, "invalid_request_error")
			require.Zero(t, env.virtual.CallCount(), "invalid roles must fail before routing or failover")
		})
	}
}
