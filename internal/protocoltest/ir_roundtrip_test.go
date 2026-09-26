package protocoltest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/anthropicbridge"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/openaibridge"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/responsesbridge"
)

// H3: IR round-trip fidelity (.design/protocol-stage-v2.md §2.4).
//
// Beta is the Tool Round Stage's IR. A request enters the IR only when the
// Stage has work (MCP, Guardrails); for the four OpenAI pairs below that means
// a round trip - source -> Beta -> target - where the direct pipeline has
// none (same protocol) or one direct bridge. Each case compares, IR against
// direct, what the provider receives and what the client receives. Every
// difference is a loss of the IR and is registered as a known gap until the
// IR carries it; OpenAI-source cutovers wait for these to be cleared or
// explicitly accepted.
//
// Values are compared as canonical JSON with IDs and timestamps normalized
// (as the golden snapshots do) and with absent, null and zero values treated
// as equal: those are serialization artifacts of SDK types, not losses.

type irPair struct {
	source, target protocol.APIType
}

func (p irPair) String() string { return fmt.Sprintf("%s->%s", p.source, p.target) }

var irPairs = []irPair{
	{protocol.TypeOpenAIChat, protocol.TypeOpenAIChat},
	{protocol.TypeOpenAIResponses, protocol.TypeOpenAIResponses},
	{protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses},
	{protocol.TypeOpenAIResponses, protocol.TypeOpenAIChat},
}

// irPipelines returns the direct and IR pipelines for pair over terminal.
func irPipelines(t *testing.T, pair irPair, terminal stage.Endpoint) (direct, ir stage.Endpoint) {
	t.Helper()
	var err error
	switch {
	case pair.source == pair.target:
		direct = terminal
	case pair.source == protocol.TypeOpenAIChat:
		direct, err = stage.Adapt(terminal, openaibridge.NewChatToOpenAIResponses(openaibridge.ResponsesOptions{}))
	default:
		direct, err = stage.Adapt(terminal, responsesbridge.NewToOpenAIChat(responsesbridge.ChatOptions{}))
	}
	require.NoError(t, err)

	var lower stage.Bridge
	if pair.target == protocol.TypeOpenAIChat {
		lower = anthropicbridge.NewBetaToOpenAIChat(anthropicbridge.ChatOptions{})
	} else {
		lower = anthropicbridge.NewBetaToOpenAIResponses(anthropicbridge.ResponsesOptions{})
	}
	var raise stage.Bridge
	if pair.source == protocol.TypeOpenAIChat {
		raise = openaibridge.NewChatToAnthropicBeta(openaibridge.AnthropicOptions{})
	} else {
		raise = responsesbridge.NewToAnthropicBeta(responsesbridge.AnthropicOptions{})
	}
	beta, err := stage.Adapt(terminal, lower)
	require.NoError(t, err)
	ir, err = stage.Adapt(beta, raise)
	require.NoError(t, err)
	return direct, ir
}

// ─── request corpus ────────────────────────────────────────────────────────

var irChatRequests = map[string]string{
	"basic": `{"model":"m","messages":[
		{"role":"system","content":"Be brief."},
		{"role":"user","content":"What is the capital of France?"}]}`,
	"sampling": `{"model":"m","messages":[{"role":"user","content":"hi"}],
		"temperature":0.2,"top_p":0.9,"max_completion_tokens":256,"stop":["END"],
		"seed":7,"presence_penalty":0.1,"frequency_penalty":0.2,"user":"u-1"}`,
	"tools": `{"model":"m","messages":[{"role":"user","content":"weather?"}],
		"tools":[{"type":"function","function":{"name":"get_weather","description":"w",
			"parameters":{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}}}],
		"tool_choice":"auto","parallel_tool_calls":false}`,
	"tool_history": `{"model":"m","messages":[
		{"role":"user","content":"weather in Paris?"},
		{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"location\":\"Paris\"}"}}]},
		{"role":"tool","tool_call_id":"call_1","content":"18C"}],
		"tools":[{"type":"function","function":{"name":"get_weather","parameters":{"type":"object","properties":{"location":{"type":"string"}}}}}]}`,
	"image": `{"model":"m","messages":[{"role":"user","content":[
		{"type":"text","text":"describe"},
		{"type":"image_url","image_url":{"url":"data:image/png;base64,iVBORw0KGgo="}}]}]}`,
	"response_format": `{"model":"m","messages":[{"role":"user","content":"json please"}],
		"response_format":{"type":"json_schema","json_schema":{"name":"answer","schema":{"type":"object","properties":{"a":{"type":"string"}}},"strict":true}}}`,
	"reasoning": `{"model":"m","messages":[{"role":"user","content":"think"}],"reasoning_effort":"high"}`,
	"developer": `{"model":"m","messages":[{"role":"developer","content":"Follow policy."},{"role":"user","content":"hi"}]}`,
}

var irResponsesRequests = map[string]string{
	"basic": `{"model":"m","instructions":"Be brief.","input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"What is the capital of France?"}]}]}`,
	"string_input": `{"model":"m","input":"What is the capital of France?"}`,
	"sampling":     `{"model":"m","input":"hi","temperature":0.2,"top_p":0.9,"max_output_tokens":256,"user":"u-1"}`,
	"tools": `{"model":"m","input":"weather?",
		"tools":[{"type":"function","name":"get_weather","description":"w",
			"parameters":{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]},"strict":false}],
		"tool_choice":"auto","parallel_tool_calls":false}`,
	"tool_history": `{"model":"m","input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"weather in Paris?"}]},
		{"type":"function_call","call_id":"call_1","name":"get_weather","arguments":"{\"location\":\"Paris\"}"},
		{"type":"function_call_output","call_id":"call_1","output":"18C"}],
		"tools":[{"type":"function","name":"get_weather","parameters":{"type":"object","properties":{"location":{"type":"string"}}},"strict":false}]}`,
	"image": `{"model":"m","input":[{"type":"message","role":"user","content":[
		{"type":"input_text","text":"describe"},
		{"type":"input_image","image_url":"data:image/png;base64,iVBORw0KGgo=","detail":"auto"}]}]}`,
	"text_format": `{"model":"m","input":"json please",
		"text":{"format":{"type":"json_schema","name":"answer","schema":{"type":"object","properties":{"a":{"type":"string"}}},"strict":true}}}`,
	"reasoning": `{"model":"m","input":"think","reasoning":{"effort":"high"}}`,
}

func irRequest(t *testing.T, source protocol.APIType, raw string) any {
	t.Helper()
	switch source {
	case protocol.TypeOpenAIChat:
		var request openai.ChatCompletionNewParams
		require.NoError(t, json.Unmarshal([]byte(raw), &request))
		return &request
	default:
		var request responses.ResponseNewParams
		require.NoError(t, json.Unmarshal([]byte(raw), &request))
		return &request
	}
}

func irRequestCorpus(source protocol.APIType) map[string]string {
	if source == protocol.TypeOpenAIChat {
		return irChatRequests
	}
	return irResponsesRequests
}

// ─── comparison ────────────────────────────────────────────────────────────

// irCanonical parses wire JSON into a value with minted IDs and timestamps
// normalized, equivalent shapes unified, the model removed and zero values
// dropped.
func irCanonical(raw string) any {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return raw
	}
	return dropZero(dropModel(equivalentShapes(normalizeMinted(v))))
}

// mintedID matches identifiers a gateway or provider generates per call
// (prefix_ plus a long random hex or UUID tail). Fixture IDs such as
// "call-validate-weather" do not match, so an ID the IR replaces instead of
// carrying through shows up as a difference.
var mintedID = regexp.MustCompile(`^[a-z]+[_-]?[0-9a-f]{16,}$|^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

var timestampKeys = map[string]bool{"created": true, "created_at": true, "completed_at": true, "obfuscation": true, "sequence_number": true}

func normalizeMinted(v any) any {
	switch value := v.(type) {
	case map[string]any:
		for k, item := range value {
			if timestampKeys[k] {
				delete(value, k)
				continue
			}
			value[k] = normalizeMinted(item)
		}
	case []any:
		for i, item := range value {
			value[i] = normalizeMinted(item)
		}
	case string:
		if mintedID.MatchString(strings.ReplaceAll(value, "-", "")) || mintedID.MatchString(value) {
			return "<MINTED>"
		}
	}
	return v
}

// equivalentShapes unifies request shapes that differ on the wire but mean
// the same thing to every provider; anything not listed here is compared as
// is. The rules:
//   - a message's content given as a string is its single text part
//     (Chat {type:text}, Responses {type:input_text|output_text});
//   - a Responses input given as a string is one user message;
//   - a function_call_output's output given as a string is one input_text;
//   - tool_choice "auto" is the default when tools are offered;
//   - status "completed" on a Responses input or output item is the default;
//   - a text part with empty text is no part.
func equivalentShapes(v any) any {
	request, ok := v.(map[string]any)
	if !ok {
		return v
	}
	if choice, ok := request["tool_choice"].(string); ok && choice == "auto" {
		delete(request, "tool_choice")
	}
	if input, ok := request["input"].(string); ok {
		request["input"] = []any{map[string]any{"type": "message", "role": "user", "content": input}}
	}
	if output, ok := request["output"].([]any); ok {
		for _, raw := range output {
			if item, ok := raw.(map[string]any); ok && item["status"] == "completed" {
				delete(item, "status")
			}
		}
	}
	for _, key := range []string{"messages", "input"} {
		items, _ := request[key].([]any)
		for _, raw := range items {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if status, ok := item["status"].(string); ok && status == "completed" {
				delete(item, "status")
			}
			if output, ok := item["output"].(string); ok {
				item["output"] = []any{map[string]any{"type": "input_text", "text": output}}
			}
			if text, ok := item["content"].(string); ok {
				partType := "text"
				if key == "input" {
					partType = "input_text"
					if item["role"] == "assistant" {
						partType = "output_text"
					}
				}
				item["content"] = []any{map[string]any{"type": partType, "text": text}}
			}
			if parts, ok := item["content"].([]any); ok {
				kept := make([]any, 0, len(parts))
				for _, part := range parts {
					if p, ok := part.(map[string]any); ok && p["type"] == "text" && (p["text"] == nil || p["text"] == "") {
						continue
					}
					kept = append(kept, part)
				}
				item["content"] = kept
			}
		}
	}
	return request
}

func dropZero(v any) any {
	switch value := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, item := range value {
			if cleaned := dropZero(item); cleaned != nil {
				out[k] = cleaned
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case []any:
		out := make([]any, 0, len(value))
		for _, item := range value {
			out = append(out, dropZero(item))
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case string:
		if value == "" {
			return nil
		}
	case float64:
		if value == 0 {
			return nil
		}
	case bool:
		if !value {
			return nil
		}
	}
	return v
}

func flatten(prefix string, v any, out map[string]string) {
	switch value := v.(type) {
	case map[string]any:
		for k, item := range value {
			flatten(prefix+"."+k, item, out)
		}
	case []any:
		for i, item := range value {
			flatten(fmt.Sprintf("%s[%d]", prefix, i), item, out)
		}
	default:
		data, _ := json.Marshal(value)
		out[prefix] = string(data)
	}
}

// irDiff lists the paths where ir differs from direct.
func irDiff(what, direct, ir string) []string {
	d, i := map[string]string{}, map[string]string{}
	flatten(what, irCanonical(direct), d)
	flatten(what, irCanonical(ir), i)
	keys := map[string]bool{}
	for k := range d {
		keys[k] = true
	}
	for k := range i {
		keys[k] = true
	}
	var paths []string
	for k := range keys {
		if d[k] != i[k] {
			paths = append(paths, k)
		}
	}
	sort.Strings(paths)
	var diffs []string
	for _, k := range paths {
		dv, iv := d[k], i[k]
		if dv == "" {
			dv = "(absent)"
		}
		if iv == "" {
			iv = "(absent)"
		}
		diffs = append(diffs, fmt.Sprintf("%s: direct=%s ir=%s", k, truncate(dv, 120), truncate(iv, 120)))
	}
	return diffs
}

func irWire(t *testing.T, value any) string {
	t.Helper()
	data, err := wireJSON(value)
	require.NoError(t, err)
	return data
}

// irRun sends request through endpoint and returns the provider request and
// the client-visible answer (the message, or one event per line).
func irRun(t *testing.T, endpoint stage.Endpoint, terminal *recordingEndpoint, request any, streaming bool) (string, string) {
	t.Helper()
	before := len(terminal.requests)
	call := stage.Call{Request: request}
	var answer string
	if !streaming {
		response, err := endpoint.Complete(context.Background(), call)
		require.NoError(t, err)
		answer = irWire(t, response.Value)
	} else {
		events, err := endpoint.Stream(context.Background(), call)
		require.NoError(t, err)
		var lines []string
		for {
			event, err := events.Next(context.Background())
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(t, err)
			lines = append(lines, irWire(t, event.Value))
		}
		require.NoError(t, events.Close())
		answer = "[" + strings.Join(lines, ",") + "]"
	}
	require.Len(t, terminal.requests, before+1)
	return terminal.requests[before], answer
}

// ─── tests ─────────────────────────────────────────────────────────────────

// TestIRRoundTripRequest: the provider receives the same request whether it
// went through the IR or not.
func TestIRRoundTripRequest(t *testing.T) {
	t.Parallel()
	s := TextScenario()
	for _, pair := range irPairs {
		corpus := irRequestCorpus(pair.source)
		names := make([]string, 0, len(corpus))
		for name := range corpus {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			pair, name := pair, name
			t.Run(pair.String()+"/"+name, func(t *testing.T) {
				t.Parallel()
				terminal := &recordingEndpoint{Endpoint: &fixtureEndpoint{protocol: pair.target, mock: s.MockResponses[targetFormat(pair.target)]}}
				direct, ir := irPipelines(t, pair, terminal)
				directRequest, _ := irRun(t, direct, terminal, irRequest(t, pair.source, corpus[name]), false)
				irRequestWire, _ := irRun(t, ir, terminal, irRequest(t, pair.source, corpus[name]), false)
				checkCase(t, t.Name(), irDiff("request", directRequest, irRequestWire),
					"direct: "+directRequest+"\nir:     "+irRequestWire)
			})
		}
	}
}

// TestIRRoundTripResponse: the client receives the same answer, complete and
// streamed, whether it went through the IR or not.
func TestIRRoundTripResponse(t *testing.T) {
	t.Parallel()
	for _, pair := range irPairs {
		for _, s := range AllScenarios() {
			for _, streaming := range []bool{false, true} {
				pair, s, streaming := pair, s, streaming
				t.Run(fmt.Sprintf("%s/%s/%s", pair, s.Name, streamMode(streaming)), func(t *testing.T) {
					t.Parallel()
					mock, ok := s.MockResponses[targetFormat(pair.target)]
					if !ok || hasTag(s, "error") {
						t.Skip("no success fixture for the target format")
					}
					if reason, skip := streamingSkipReason(s, streaming); skip {
						t.Skip(reason)
					}
					terminal := &recordingEndpoint{Endpoint: &fixtureEndpoint{protocol: pair.target, mock: mock}}
					direct, ir := irPipelines(t, pair, terminal)
					basic := irRequestCorpus(pair.source)["basic"]
					_, directAnswer := irRun(t, direct, terminal, irRequest(t, pair.source, basic), streaming)
					_, irAnswer := irRun(t, ir, terminal, irRequest(t, pair.source, basic), streaming)
					var diffs []string
					if streaming {
						diffs = append(irDiff("stream", irAssembled(t, pair.source, directAnswer), irAssembled(t, pair.source, irAnswer)),
							irMissingEvents(pair.source, directAnswer, irAnswer)...)
					} else {
						diffs = irDiff("response", directAnswer, irAnswer)
					}
					checkCase(t, t.Name(), diffs,
						"direct: "+truncate(directAnswer, 2000)+"\nir:     "+truncate(irAnswer, 2000))
				})
			}
		}
	}
}

func hasTag(s Scenario, tag string) bool {
	for _, candidate := range s.Tags {
		if candidate == tag {
			return true
		}
	}
	return false
}

// irAssembled assembles a client stream (a JSON array of events) into the
// semantic answer the client ends up with: text, thinking, tool calls, finish
// reason and usage.
func irAssembled(t *testing.T, source protocol.APIType, answer string) string {
	t.Helper()
	var events []json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(answer), &events))
	lines := make([]string, len(events))
	for i, event := range events {
		lines[i] = "data: " + string(event)
	}
	var result RoundTripResult
	fillFromParsedResult(&result, assembleFromEvents(lines, sourceToStyle(source)))
	calls := make([]map[string]any, len(result.ToolCalls))
	for i, call := range result.ToolCalls {
		calls[i] = map[string]any{"id": call.ID, "name": call.Name, "arguments": call.Arguments}
	}
	data, err := json.Marshal(map[string]any{
		"content": result.Content, "thinking": result.ThinkingContent, "tool_calls": calls,
		"finish_reason": result.FinishReason, "usage": result.Usage,
	})
	require.NoError(t, err)
	return string(data)
}

// irMissingEvents reports Responses stream event types the direct answer
// has and the IR answer lacks, in order. The IR may add events the protocol
// defines (in_progress, content_part.*): the harness fixtures abbreviate the
// sequence, so only missing events are losses. Chat chunks carry no type.
func irMissingEvents(source protocol.APIType, direct, ir string) []string {
	if source != protocol.TypeOpenAIResponses {
		return nil
	}
	types := func(answer string) []string {
		var events []map[string]any
		_ = json.Unmarshal([]byte(answer), &events)
		out := make([]string, len(events))
		for i, event := range events {
			out[i], _ = event["type"].(string)
		}
		return out
	}
	have := types(ir)
	var missing []string
	next := 0
	for _, want := range types(direct) {
		found := false
		for next < len(have) {
			next++
			if have[next-1] == want {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, "events: missing "+want)
		}
	}
	return missing
}

// dropModel removes every "model" field: the client edge rewrites it to the
// model the client asked for, so it is not a property of the IR.
func dropModel(v any) any {
	switch value := v.(type) {
	case map[string]any:
		delete(value, "model")
		for k, item := range value {
			value[k] = dropModel(item)
		}
	case []any:
		for i, item := range value {
			value[i] = dropModel(item)
		}
	}
	return v
}
