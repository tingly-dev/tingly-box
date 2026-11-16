package protocoltest

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// This is ordinary /responses coverage; standalone /responses/compact and its
// compact-to-continue harness scenario remain separate deferred work.
func TestNativeResponsesCompactionLifecycle(t *testing.T) {
	for _, codex := range []bool{false, true} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("codex=%t/stream=%t", codex, streaming), func(t *testing.T) {
				env := NewTestEnv(t)
				defer env.Close()
				const item = `{"type":"compaction","id":"cmp_next","encrypted_content":"native-next","future_item":{"keep":true}}`
				const result = `{"id":"resp_native","object":"response","model":"provider","status":"completed","output":[` + item + `],"usage":{"input_tokens":20,"output_tokens":5}}`
				s := TextScenario()
				s.Name = "native_state_lifecycle"
				s.MockResponses[FormatOpenAIResponses] = MockResponseBuilder{
					NonStream: func() (int, []byte) { return 200, []byte(result) },
					Stream: func() []string {
						return []string{
							`data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_native","status":"in_progress","output":[]}}`,
							`data: {"type":"response.compaction.compacting","sequence_number":1,"future_event":{"keep":true}}`,
							`data: {"type":"response.output_item.added","sequence_number":2,"output_index":0,"item":` + item + `}`,
							`data: {"type":"response.output_item.done","sequence_number":3,"output_index":0,"item":` + item + `}`,
							`data: {"type":"response.completed","sequence_number":4,"response":` + result + `}`,
						}
					},
				}
				if codex {
					env.SetupCodexAssemblyRoute(protocol.TypeOpenAIResponses, s)
				} else {
					env.SetupRoute(protocol.TypeOpenAIResponses, protocol.TypeOpenAIResponses, s)
				}
				model := env.routeModels[routeKey(protocol.TypeOpenAIResponses, protocol.TypeOpenAIResponses, s.Name)]
				status, body := sendRaw(t, env, "/tingly/openai/v1/responses", []byte(`{"model":"`+model+`","stream":`+fmt.Sprint(streaming)+`,"context_management":[{"type":"compaction","compact_threshold":12345}],"input":[{"type":"message","role":"user","content":"continue"},{"type":"compaction","encrypted_content":"native-prior"}]}`))
				require.Equal(t, 200, status, body)
				capture := env.UpstreamLastRequest(EndpointResponses)
				require.NotNil(t, capture)
				require.Equal(t, "compaction", gjson.GetBytes(capture.Body, "context_management.0.type").String())
				require.Equal(t, int64(12345), gjson.GetBytes(capture.Body, "context_management.0.compact_threshold").Int())
				require.Equal(t, "native-prior", gjson.GetBytes(capture.Body, "input.1.encrypted_content").String())
				var output gjson.Result
				if streaming {
					var types []string
					for _, line := range strings.Split(body, "\n") {
						if !strings.HasPrefix(line, "data: ") {
							continue
						}
						event := gjson.Parse(strings.TrimPrefix(line, "data: "))
						types = append(types, event.Get("type").String())
						if event.Get("type").String() == "response.compaction.compacting" {
							require.True(t, event.Get("future_event.keep").Bool())
						}
						if event.Get("type").String() == "response.output_item.done" {
							require.Equal(t, "native-next", event.Get("item.encrypted_content").String())
							require.True(t, event.Get("item.future_item.keep").Bool())
						}
						if event.Get("type").String() == "response.completed" {
							output = event.Get("response.output.0")
						}
					}
					require.Equal(t, []string{"response.created", "response.compaction.compacting", "response.output_item.added", "response.output_item.done", "response.completed"}, types)
				} else {
					output = gjson.Get(body, "output.0")
				}
				require.Equal(t, "compaction", output.Get("type").String())
				require.Equal(t, "native-next", output.Get("encrypted_content").String())
				require.True(t, output.Get("future_item.keep").Bool())
				// Resume on the same native backend, keeping the policy and opaque state.
				replay, err := json.Marshal(map[string]any{"model": model, "stream": streaming, "input": []any{json.RawMessage(output.Raw), map[string]any{"type": "message", "role": "user", "content": "next turn"}}, "context_management": []any{map[string]any{"type": "compaction", "compact_threshold": 12345}}})
				require.NoError(t, err)
				status, body = sendRaw(t, env, "/tingly/openai/v1/responses", replay)
				require.Equal(t, 200, status, body)
				capture = env.UpstreamLastRequest(EndpointResponses)
				require.Equal(t, "native-next", gjson.GetBytes(capture.Body, "input.0.encrypted_content").String())
				require.Equal(t, "next turn", gjson.GetBytes(capture.Body, "input.1.content").String())
				require.Equal(t, int64(12345), gjson.GetBytes(capture.Body, "context_management.0.compact_threshold").Int())
			})
		}
	}
}

func TestNativeCodexCompactionTrigger(t *testing.T) {
	for _, fails := range []bool{false, true} {
		t.Run(fmt.Sprint(fails), func(t *testing.T) {
			env := NewTestEnv(t)
			defer env.Close()
			s := TextScenario()
			s.Name = "native_v2"
			s.MockResponses[FormatOpenAIResponses] = MockResponseBuilder{Stream: func() []string {
				if fails {
					return []string{`data: {"error":{"message":"response protection is unavailable","type":"internal_error"}}`}
				}
				return []string{
					`data: {"type":"response.created","response":{"id":"resp_native","status":"in_progress","output":[]}}`,
					`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"compaction","id":"cmp_native","encrypted_content":"native-cipher"}}`,
					`data: {"type":"response.completed","response":{"id":"resp_native","status":"completed","output":[{"type":"compaction","id":"cmp_native","encrypted_content":"native-cipher"}],"usage":{"input_tokens":20,"output_tokens":5}}}`,
				}
			}}
			env.SetupCodexAssemblyRoute(protocol.TypeOpenAIResponses, s)
			model := env.routeModels[routeKey(protocol.TypeOpenAIResponses, protocol.TypeOpenAIResponses, s.Name)]
			status, result := sendRaw(t, env, "/tingly/openai/v1/responses", []byte(`{"model":"`+model+`","stream":true,"tools":[{"type":"web_search"}],"input":[{"type":"message","role":"user","content":"Fix auth"},{"type":"compaction_trigger"}]}`))
			if fails {
				require.GreaterOrEqual(t, status, 400, result)
				require.Contains(t, result, "response protection is unavailable")
				require.NotContains(t, result, "response.completed")
			} else {
				require.Equal(t, 200, status, result)
				require.Contains(t, result, "native-cipher")
				require.Contains(t, result, "response.completed")
			}
			capture := env.UpstreamLastRequest(EndpointResponses)
			require.NotNil(t, capture)
			require.True(t, gjson.GetBytes(capture.Body, "stream").Bool())
			require.Equal(t, "compaction_trigger", gjson.GetBytes(capture.Body, "input.1.type").String())
			require.Equal(t, "web_search", gjson.GetBytes(capture.Body, "tools.0.type").String())
			require.NotContains(t, result, "tingly-box:summary:")
		})
	}
}
