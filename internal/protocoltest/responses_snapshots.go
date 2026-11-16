package protocoltest

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
)

func responsesSnapshotFixture(mode string) Scenario {
	const args = `{"id":9007199254740993}`
	item := map[string]any{"type": "function_call", "id": "fc_1", "call_id": "call_1", "name": "lookup", "arguments": args}
	added := map[string]any{"type": "function_call", "id": "fc_1", "call_id": "call_1", "name": "lookup", "arguments": ""}
	message := map[string]any{"type": "message", "id": "msg_1", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "hello"}}}
	response := map[string]any{"id": "resp_1", "object": "response", "status": "completed", "output": []any{}, "usage": map[string]any{"input_tokens": 10, "output_tokens": 7, "total_tokens": 17}}
	events := []map[string]any{{"type": "response.created", "response": map[string]any{"id": "resp_1", "status": "in_progress", "output": []any{}}}}
	switch mode {
	case "arguments_done", "item_done", "completed_only", "partial_arguments", "repeated_snapshots", "initial_arguments", "truncated_tool":
		if mode != "completed_only" {
			if mode == "initial_arguments" {
				added["arguments"] = args
			}
			events = append(events, map[string]any{"type": "response.output_item.added", "output_index": 2, "item": added})
		}
		if mode == "partial_arguments" || mode == "repeated_snapshots" {
			delta := args
			if mode == "partial_arguments" {
				delta = `{"id":`
			}
			events = append(events, map[string]any{"type": "response.function_call_arguments.delta", "output_index": 2, "item_id": "fc_1", "delta": delta})
		}
		if mode == "arguments_done" || mode == "repeated_snapshots" {
			events = append(events, map[string]any{"type": "response.function_call_arguments.done", "output_index": 2, "item_id": "fc_1", "arguments": args})
		}
		if mode == "item_done" || mode == "repeated_snapshots" {
			events = append(events, map[string]any{"type": "response.output_item.done", "output_index": 2, "item": item})
		}
		// A final array uses its own contiguous output positions. Supply preceding
		// items when the tool's streamed output_index is 2.
		if mode == "completed_only" {
			response["output"] = []any{item}
		} else if mode == "partial_arguments" || mode == "repeated_snapshots" || mode == "truncated_tool" {
			response["output"] = []any{map[string]any{"type": "reasoning", "id": "rs_empty", "summary": []any{}}, map[string]any{"type": "message", "id": "msg_empty", "content": []any{}}, item}
		}
		if mode == "truncated_tool" {
			response["status"] = "incomplete"
			response["incomplete_details"] = map[string]any{"reason": "max_output_tokens"}
		}
	case "text_part_done":
		events = append(events, map[string]any{"type": "response.output_text.delta", "output_index": 0, "content_index": 0, "delta": "hel"}, map[string]any{"type": "response.content_part.done", "output_index": 0, "content_index": 0, "part": map[string]any{"type": "output_text", "text": "hello"}})
	case "refusal_part_done":
		events = append(events, map[string]any{"type": "response.content_part.done", "output_index": 0, "content_index": 0, "part": map[string]any{"type": "refusal", "refusal": "cannot comply"}})
	case "summary_part_done":
		events = append(events, map[string]any{"type": "response.reasoning_summary_part.done", "output_index": 0, "summary_index": 0, "part": map[string]any{"type": "summary_text", "text": "visible summary"}})
	case "mixed_final":
		events = append(events, map[string]any{"type": "response.output_text.delta", "output_index": 0, "content_index": 0, "delta": "hel"})
		response["output"] = []any{message, map[string]any{"type": "message", "id": "msg_2", "content": []any{map[string]any{"type": "output_text", "text": " world"}, map[string]any{"type": "refusal", "refusal": "cannot comply"}}}, map[string]any{"type": "reasoning", "id": "rs_1", "summary": []any{map[string]any{"type": "summary_text", "text": "visible summary"}}}, item}
	}
	kind := "response.completed"
	if mode == "truncated_tool" {
		kind = "response.incomplete"
	}
	events = append(events, map[string]any{"type": kind, "response": response})
	return Scenario{Name: "snapshots_" + mode, MockResponses: map[ResponseFormat]MockResponseBuilder{FormatOpenAIResponses: {
		Stream: func() []string {
			var lines []string
			for i, e := range events {
				e["sequence_number"] = i
				lines = append(lines, "event: "+e["type"].(string), "data: "+string(mustMarshal(e)))
			}
			return lines
		},
		NonStream: func() (int, []byte) { return 200, mustMarshal(response) },
	}}}
}

func runResponsesSnapshot(t flagTB, env *TestEnv, source protocol.APIType, mode string) {
	t.Helper()
	fixture := responsesSnapshotFixture(mode)
	env.SetupRoute(source, protocol.TypeOpenAIResponses, fixture)
	res, err := env.SendAsCLI(source, protocol.TypeOpenAIResponses, fixture, true)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if res.HTTPStatus != 200 {
		t.Fatalf("HTTP %d: %s", res.HTTPStatus, res.RawBody)
	}
	if env.virtual.EndpointHits(EndpointResponses) != 1 {
		t.Fatalf("request did not reach Responses upstream exactly once")
	}
	var text, refusal, reasoning, args strings.Builder
	terminals, starts := 0, 0
	open := map[int]bool{}
	finish := ""
	for _, line := range res.StreamEvents {
		payload, ok := sse.ParseSSEDataPayload(line)
		if !ok || payload == "[DONE]" {
			continue
		}
		r := gjson.Parse(payload)
		if r.Get("error").Exists() {
			t.Errorf("client stream error: %s", payload)
		}
		if source == protocol.TypeOpenAIChat {
			for _, choice := range r.Get("choices").Array() {
				d := choice.Get("delta")
				text.WriteString(d.Get("content").String())
				refusal.WriteString(d.Get("refusal").String())
				reasoning.WriteString(d.Get("reasoning_content").String())
				for _, call := range d.Get("tool_calls").Array() {
					retentionCheck(t, call.Get("index").String(), "0", "dense Chat tool index")
					args.WriteString(call.Get("function.arguments").String())
					if call.Get("id").String() != "" {
						starts++
						retentionCheck(t, call.Get("id").String(), "call_1", "tool correlation")
						retentionCheck(t, call.Get("function.name").String(), "lookup", "tool name")
					}
				}
				if f := choice.Get("finish_reason").String(); f != "" {
					terminals++
					finish = f
					retentionCheck(t, r.Get("usage.completion_tokens").String(), "7", "usage")
				}
			}
		} else {
			idx := int(r.Get("index").Int())
			switch r.Get("type").String() {
			case "content_block_start":
				if open[idx] {
					t.Errorf("block %d started twice", idx)
				}
				open[idx] = true
				if r.Get("content_block.type").String() == "tool_use" {
					starts++
					retentionCheck(t, r.Get("content_block.id").String(), "call_1", "tool correlation")
				}
			case "content_block_delta":
				if !open[idx] {
					t.Errorf("delta outside open block %d", idx)
				}
				text.WriteString(r.Get("delta.text").String())
				reasoning.WriteString(r.Get("delta.thinking").String())
				args.WriteString(r.Get("delta.partial_json").String())
			case "content_block_stop":
				if !open[idx] {
					t.Errorf("stop outside open block %d", idx)
				}
				delete(open, idx)
			case "message_delta":
				finish = r.Get("delta.stop_reason").String()
				retentionCheck(t, r.Get("usage.output_tokens").String(), "7", "usage")
			case "message_stop":
				terminals++
			}
		}
	}
	retentionCheck(t, fmt.Sprint(terminals), "1", "terminal count")
	if len(open) != 0 {
		t.Errorf("unclosed Anthropic blocks: %v", open)
	}
	wantText, wantRefusal, wantReason, wantArgs := "", "", "", ""
	switch mode {
	case "text_part_done":
		wantText = "hello"
	case "refusal_part_done":
		wantRefusal = "cannot comply"
	case "summary_part_done":
		wantReason = "visible summary"
	case "mixed_final":
		wantText = "hello world"
		wantRefusal = "cannot comply"
		wantReason = "visible summary"
		wantArgs = `{"id":9007199254740993}`
	default:
		wantArgs = `{"id":9007199254740993}`
	}
	if source != protocol.TypeOpenAIChat {
		wantText += wantRefusal
		wantRefusal = ""
	}
	retentionCheck(t, text.String(), wantText, "text")
	retentionCheck(t, refusal.String(), wantRefusal, "refusal")
	retentionCheck(t, reasoning.String(), wantReason, "reasoning")
	retentionCheck(t, args.String(), wantArgs, "exact tool arguments")
	wantFinish := "stop"
	if source != protocol.TypeOpenAIChat {
		wantFinish = "end_turn"
	}
	if wantArgs != "" {
		retentionCheck(t, fmt.Sprint(starts), "1", "tool start count")
		wantFinish = "tool_calls"
		if source != protocol.TypeOpenAIChat {
			wantFinish = "tool_use"
		}
	}
	if mode == "truncated_tool" {
		wantFinish = "length"
		if source != protocol.TypeOpenAIChat {
			wantFinish = "max_tokens"
		}
	}
	retentionCheck(t, finish, wantFinish, "finish reason")
}

func responsesSnapshotCases() []recorderCase {
	var cases []recorderCase
	for _, source := range []protocol.APIType{protocol.TypeOpenAIChat, protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta} {
		for _, mode := range []string{"arguments_done", "item_done", "completed_only", "partial_arguments", "repeated_snapshots", "initial_arguments", "truncated_tool", "text_part_done", "refusal_part_done", "summary_part_done", "mixed_final"} {
			cases = append(cases, recorderCase{name: fmt.Sprintf("response_retention/responses/%s/%s", source, mode), scenario: mode, source: source, target: protocol.TypeOpenAIResponses, streaming: true, run: func(t flagTB, env *TestEnv) { runResponsesSnapshot(t, env, source, mode) }})
		}
	}
	return cases
}
