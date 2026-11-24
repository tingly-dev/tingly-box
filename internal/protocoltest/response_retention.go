package protocoltest

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
)

// These cases inspect raw client wire values, rather than the normalized result:
// unmarshalling tool input into float64 would hide the precision loss we test.
type retentionFixture struct{ name, initial, delta, want, stop string }

func retentionFixtures() []retentionFixture {
	return []retentionFixture{
		{"initial_tool_input", `{"id":9007199254740993}`, "", `{"id":9007199254740993}`, "tool_use"},
		{"empty_tool_input", `{}`, "", `{}`, "tool_use"},
		{"delta_replaces_initial", `{"id":1}`, `{"id":9007199254740995}`, `{"id":9007199254740995}`, "tool_use"},
		{"truncated_tool", `{}`, `{"id":9007199254740993}`, `{"id":9007199254740993}`, "max_tokens"},
		{"initial_text", "", "", "", "end_turn"},
	}
}

func retentionScenario(f retentionFixture) Scenario {
	block := `{"type":"text","text":"prefix"}`
	if f.initial != "" {
		block = `{"type":"tool_use","id":"tool_1","name":"lookup","input":` + f.initial + `}`
	}
	events := []string{
		`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-test","content":[],"usage":{"input_tokens":10,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":` + block + `}`,
	}
	if f.initial == "" {
		thinking := []string{
			`{"type":"content_block_start","index":9,"content_block":{"type":"thinking","thinking":"visible","signature":"opaque"}}`,
			`{"type":"content_block_delta","index":9,"delta":{"type":"thinking_delta","thinking":" thought"}}`,
			`{"type":"content_block_stop","index":9}`,
		}
		events = append(append(append([]string{}, events[:1]...), thinking...), events[1:]...)
	}
	if f.delta != "" { // Split the parameter JSON across actual upstream SSE frames.
		split := len(f.delta) / 2
		for _, part := range []string{f.delta[:split], f.delta[split:]} {
			events = append(events, fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":%q}}`, part))
		}
	}
	if f.initial == "" {
		events = append(events, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" tail"}}`)
	}
	events = append(events, `{"type":"content_block_stop","index":0}`,
		fmt.Sprintf(`{"type":"message_delta","delta":{"stop_reason":%q},"usage":{"output_tokens":4}}`, f.stop),
		`{"type":"message_delta","delta":{},"usage":{"output_tokens":7}}`,
		`{"type":"message_stop"}`)
	finalBlock := block
	if f.initial != "" {
		finalBlock = `{"type":"tool_use","id":"tool_1","name":"lookup","input":` + f.want + `}`
	} else {
		finalBlock = `{"type":"text","text":"prefix tail"}`
	}
	// Thinking is visible only on the Chat route in this suite. It is deliberately
	// separate from opaque Anthropic signatures and does not promise replayability.
	body := fmt.Sprintf(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-test","stop_reason":%q,"content":[{"type":"thinking","thinking":"visible thought","signature":"opaque"},%s],"usage":{"input_tokens":10,"output_tokens":7}}`, f.stop, finalBlock)
	return Scenario{Name: "retention_" + f.name, MockResponses: map[ResponseFormat]MockResponseBuilder{
		FormatAnthropic: {Stream: func() []string {
			var lines []string
			for _, event := range events {
				lines = append(lines, "event: "+gjson.Get(event, "type").String(), "data: "+event)
			}
			return lines
		}, NonStream: func() (int, []byte) { return 200, []byte(body) }},
	}}
}

func retentionCheck(t flagTB, got, want, field string) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %q, want %q", field, got, want)
	}
}

func runRetention(t flagTB, env *TestEnv, source, target protocol.APIType, f retentionFixture, streaming bool) {
	t.Helper()
	fixture := retentionScenario(f)
	env.SetupRoute(source, target, fixture)
	res, err := env.SendAsCLI(source, target, fixture, streaming)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if res.HTTPStatus != 200 {
		t.Fatalf("HTTP %d: %s", res.HTTPStatus, res.RawBody)
	}
	if env.virtual.EndpointHits(EndpointAnthropic) != 1 {
		t.Fatalf("expected exactly one real upstream Messages request")
	}
	var args, text strings.Builder
	var snapshots []string
	terminals := 0
	assertResponse := func(r gjson.Result) {
		terminals++
		status := "completed"
		if f.stop == "max_tokens" {
			status = "incomplete"
		}
		retentionCheck(t, r.Get("status").String(), status, "Responses status")
		if status == "incomplete" {
			retentionCheck(t, r.Get("incomplete_details.reason").String(), "max_output_tokens", "incomplete reason")
		}
		retentionCheck(t, r.Get("usage.output_tokens").String(), "7", "output usage")
		retentionCheck(t, r.Get("usage.input_tokens").String(), "10", "input usage")
		for _, item := range r.Get("output").Array() {
			if item.Get("type").String() == "function_call" {
				snapshots = append(snapshots, item.Get("arguments").String())
				retentionCheck(t, item.Get("call_id").String(), "tool_1", "call ID")
			}
			if item.Get("type").String() == "message" {
				for _, part := range item.Get("content").Array() {
					text.WriteString(part.Get("text").String())
				}
			}
		}
	}
	if !streaming {
		root := gjson.ParseBytes(res.RawBody)
		if source == protocol.TypeOpenAIChat {
			retentionCheck(t, root.Get("choices.0.message.reasoning_content").String(), "visible thought", "thinking")
			if f.initial != "" {
				args.WriteString(root.Get("choices.0.message.tool_calls.0.function.arguments").String())
			} else {
				text.WriteString(root.Get("choices.0.message.content").String())
			}
			finish := "stop"
			if f.initial != "" {
				finish = "tool_calls"
			}
			if f.stop == "max_tokens" {
				finish = "length"
			}
			retentionCheck(t, root.Get("choices.0.finish_reason").String(), finish, "finish reason")
			retentionCheck(t, root.Get("usage.completion_tokens").String(), "7", "output usage")
		} else {
			assertResponse(root)
		}
	} else {
		usageChunks := 0
		var streamedText, reasoning strings.Builder
		for _, line := range res.StreamEvents {
			payload, ok := sse.ParseSSEDataPayload(line)
			if !ok {
				continue
			}
			if payload == "[DONE]" {
				continue
			}
			root := gjson.Parse(payload)
			if root.Get("error").Exists() {
				t.Errorf("unexpected stream error: %s", payload)
			}
			if source == protocol.TypeOpenAIChat {
				for _, choice := range root.Get("choices").Array() {
					streamedText.WriteString(choice.Get("delta.content").String())
					reasoning.WriteString(choice.Get("delta.reasoning_content").String())
					for _, call := range choice.Get("delta.tool_calls").Array() {
						args.WriteString(call.Get("function.arguments").String())
					}
					if finish := choice.Get("finish_reason").String(); finish != "" {
						terminals++
						want := "tool_calls"
						if f.initial == "" {
							want = "stop"
						}
						if f.stop == "max_tokens" {
							want = "length"
						}
						retentionCheck(t, finish, want, "finish reason")
					}
				}
				if root.Get("usage").Exists() {
					usageChunks++
					retentionCheck(t, root.Get("usage.completion_tokens").String(), "7", "tail output usage")
				}
			} else {
				switch root.Get("type").String() {
				case "response.function_call_arguments.delta":
					args.WriteString(root.Get("delta").String())
				case "response.output_item.done":
					if root.Get("item.type").String() == "function_call" {
						snapshots = append(snapshots, root.Get("item.arguments").String())
					}
				case "response.function_call_arguments.done":
					snapshots = append(snapshots, root.Get("arguments").String())
				case "response.output_text.delta":
					streamedText.WriteString(root.Get("delta").String())
				case "response.output_text.done":
					retentionCheck(t, root.Get("text").String(), "prefix tail", "text done")
				case "response.completed", "response.incomplete":
					assertResponse(root.Get("response"))
				}
			}
		}
		retentionCheck(t, fmt.Sprint(terminals), "1", "terminal event count")
		if source == protocol.TypeOpenAIChat {
			retentionCheck(t, fmt.Sprint(usageChunks), "1", "usage chunk count")
		}
		if f.initial == "" {
			retentionCheck(t, streamedText.String(), "prefix tail", "streamed text")
			if source == protocol.TypeOpenAIChat {
				text.WriteString(streamedText.String())
				retentionCheck(t, reasoning.String(), "visible thought", "streamed thinking")
			}
		}
	}
	if f.initial != "" {
		if source == protocol.TypeOpenAIChat || streaming {
			retentionCheck(t, args.String(), f.want, "client argument deltas")
		}
		for i, snapshot := range snapshots {
			retentionCheck(t, snapshot, f.want, fmt.Sprintf("argument snapshot %d", i))
		}
		if source == protocol.TypeOpenAIResponses {
			want := 1
			if streaming {
				want = 3
			}
			retentionCheck(t, fmt.Sprint(len(snapshots)), fmt.Sprint(want), "argument snapshot count")
		}
	} else {
		retentionCheck(t, text.String(), "prefix tail", "final text")
	}
}

func responseRetentionCases() []recorderCase {
	var cases []recorderCase
	for _, source := range []protocol.APIType{protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses} {
		for _, target := range []protocol.APIType{protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta} {
			for _, streaming := range []bool{false, true} {
				for _, f := range retentionFixtures() {
					name := fmt.Sprintf("response_retention/%s/%s->%s/stream=%t", f.name, source, target, streaming)
					cases = append(cases, recorderCase{name: name, scenario: f.name, source: source, target: target, streaming: streaming, run: func(t flagTB, env *TestEnv) { runRetention(t, env, source, target, f, streaming) }})
				}
			}
		}
	}
	return cases
}

// ExecuteAllResponseRetention shares the exact gateway cases with go test.
func (m *Matrix) ExecuteAllResponseRetention() []TestResult {
	return m.runRecorderCases(responseRetentionCases())
}
