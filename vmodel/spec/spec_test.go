package spec_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
	"github.com/tingly-dev/tingly-box/vmodel"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark/check"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark/scenario"
	"github.com/tingly-dev/tingly-box/vmodel/openai"
	"github.com/tingly-dev/tingly-box/vmodel/spec"
)

func compile(t *testing.T, source string) *spec.Runtime {
	t.Helper()
	defs, err := spec.Decode([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := spec.New(defs)
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}
func call(t *testing.T, r http.Handler, path, body string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code, w.Body.Bytes()
}

func TestDefinitionsRoundTrip(t *testing.T) {
	for _, source := range []string{
		`{"id":"mock","kind":"mock","mock":{"content":"hello","delay":"2ms","tool_calls":[{"name":"a","arguments":{}}]}}`,
		`{"id":"wire","kind":"recording","responses":{"openai_chat":{"raw_body":"{ \"ok\": true }"}},"expect":{"http_status":200}}`,
		`{"id":"sequence","kind":"sequence","sequence":{"steps":[{"status":200},{"status":429}],"on_exhaust":"clamp","delay":"1ms"}}`,
	} {
		original, err := spec.Decode([]byte(source))
		if err != nil {
			t.Fatal(err)
		}
		for _, format := range []string{"json", "yaml"} {
			exported, err := spec.Export(original, format)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := spec.Decode(exported)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(original, restored) {
				t.Fatalf("%s round trip changed definition", format)
			}
		}
	}
}

func TestSameMockDefinitionServesAndTestsMultipleTools(t *testing.T) {
	r := compile(t, `id: multi
kind: mock
mock:
  content: working
  tool_calls:
    - {id: first, name: search, arguments: {query: test}}
    - {id: second, name: save, arguments: {value: 2}}
expect:
  http_status: 200
  tool_call_count: 2
`)
	for _, p := range []string{"/v1/chat/completions", "/anthropic/v1/messages"} {
		status, body := call(t, r, p, `{"model":"multi","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}`)
		if status != 200 || !bytes.Contains(body, []byte("search")) || !bytes.Contains(body, []byte("second")) {
			t.Fatalf("%s: %d %s", p, status, body)
		}
	}
	sc, err := r.Scenario("multi")
	if err != nil {
		t.Fatal(err)
	}
	if len(sc.Assertions) != 2 || len(sc.MockResponses) != 3 {
		t.Fatalf("matrix adapter lost assertions/protocols: %+v", sc)
	}
	status, body := sc.MockResponses[scenario.FormatOpenAIChat].NonStream()
	if status != 200 || gjson.GetBytes(body, "choices.0.message.tool_calls.#").Int() != 2 {
		t.Fatalf("adapter: %d %s", status, body)
	}
}
func TestRecordingExactBytesAndStrictDispatch(t *testing.T) {
	r := compile(t, `- id: first
  kind: recording
  responses:
    openai_chat:
      status: 202
      raw_body: '{ "value": 1 }'
- id: second
  kind: recording
  responses:
    openai_responses:
      body: {value: 2}
`)
	status, body := call(t, r, "/v1/chat/completions", `{"model":"first"}`)
	if status != 202 || string(body) != `{ "value": 1 }` {
		t.Fatalf("recording changed: %d %s", status, body)
	}
	for _, tc := range []struct {
		path, body string
		want       int
	}{
		{"/v1/chat/completions", `{"model":"missing"}`, 404},
		{"/v1/responses", `{"model":"first"}`, 404},
		{"/v1/chat/completions", `{"model":"second"}`, 404},
		{"/v1/chat/completions", `{"model":"first","stream":true}`, 400},
	} {
		if status, body := call(t, r, tc.path, tc.body); status != tc.want {
			t.Fatalf("%s: %d %s", tc.path, status, body)
		}
	}
}
func TestNativeKindsAndFreshSequence(t *testing.T) {
	source := `- id: seq
  kind: sequence
  sequence: {steps: [{status: 200, content: ok}, {status: 429}], on_exhaust: fail}
- id: choose
  kind: decision
  decision: {strategy: first, confidence: 0.9}
- id: compact
  kind: transform
  transform: {builtin: compact-round-only}
`
	r := compile(t, source)
	for _, want := range []int{200, 429, 410} {
		if status, body := call(t, r, "/v1/chat/completions", `{"model":"seq","messages":[]}`); status != want {
			t.Fatalf("sequence: %d %s want %d", status, body, want)
		}
	}
	fresh := compile(t, source)
	if status, _ := call(t, fresh, "/v1/chat/completions", `{"model":"seq","messages":[]}`); status != 200 {
		t.Fatal("sequence cursor leaked across runtimes")
	}
	status, body := call(t, r, "/v1/decisions", `{"model":"choose","state":"test","questions":{"pick":{"type":"choice","criteria":{"a":"A","b":"B"}}}}`)
	if status != 200 || gjson.GetBytes(body, "answers.pick.choice").String() != "a" {
		t.Fatalf("decision: %d %s", status, body)
	}
	status, body = call(t, r, "/v1/messages", `{"model":"compact","max_tokens":32,"messages":[{"role":"user","content":"hello"}]}`)
	if status != 200 || !bytes.Contains(body, []byte("hello")) {
		t.Fatalf("transform: %d %s", status, body)
	}
}
func TestFullInventoryRoundTrip(t *testing.T) {
	defs := spec.AllDefinitions()
	if len(defs) < len(spec.DefaultDefinitions())+12 {
		t.Fatal("test scenario inventory omitted")
	}
	for _, format := range []string{"json", "yaml"} {
		raw, err := spec.Export(defs, format)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := spec.Decode(raw)
		if err != nil {
			t.Fatal(err)
		}
		r, err := spec.New(loaded)
		if err != nil {
			t.Fatal(err)
		}
		if len(r.IDs()) != len(defs) {
			t.Fatal("catalog dropped definitions")
		}
	}
}
func TestNamedDynamicBehaviorSurvivesRequests(t *testing.T) {
	catalog := spec.NewCatalog()
	err := catalog.Register("custom/echo", func(d spec.Definition) (scenario.Scenario, error) {
		return scenario.Scenario{Name: d.ID, MockResponses: map[scenario.ResponseFormat]scenario.MockResponseBuilder{scenario.FormatOpenAIChat: {NonStreamFor: func(request []byte) (int, []byte) {
			text := gjson.GetBytes(request, "input").String()
			body, _ := json.Marshal(map[string]string{"echo": text})
			return 200, body
		}}}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defs, err := spec.Decode([]byte(`id: dynamic
kind: builtin
builtin: custom/echo
`))
	if err != nil {
		t.Fatal(err)
	}
	r, err := spec.NewWithCatalog(defs, catalog)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"one", "two"} {
		_, body := call(t, r, "/v1/chat/completions", `{"model":"dynamic","input":"`+input+`"}`)
		if gjson.GetBytes(body, "echo").String() != input {
			t.Fatalf("dynamic callback flattened: %s", body)
		}
	}
	if _, err := spec.New(defs); err == nil {
		t.Fatal("unregistered Go behavior silently accepted")
	}
	if err := catalog.RegisterModels("custom/model", func(d spec.Definition) (spec.Models, error) {
		return spec.Models{OpenAI: openai.NewMockModel(&vmodel.SharedMockSpec{ID: d.ID, Content: "from Go"})}, nil
	}); err != nil {
		t.Fatal(err)
	}
	d := spec.Definition{ID: "go", Kind: spec.Builtin, Builtin: "custom/model"}
	r, err = spec.NewWithCatalog([]spec.Definition{d}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if status, body := call(t, r, "/v1/chat/completions", `{"model":"go","messages":[]}`); status != 200 || !bytes.Contains(body, []byte("from Go")) {
		t.Fatalf("Go adapter: %d %s", status, body)
	}
}
func TestProtocolRestrictionsAndSSE(t *testing.T) {
	r := compile(t, `id: wire
kind: recording
protocols: [anthropic]
responses:
  anthropic:
    stream:
      - 'event: content_block_delta'
      - 'data: {"type":"content_block_delta","delta":{"text":"partial"}}'
`)
	server := httptest.NewServer(r)
	defer server.Close()
	resp, err := server.Client().Post(server.URL+"/anthropic/v1/messages", "application/json", strings.NewReader(`{"model":"wire","stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !bytes.Contains(body, []byte("partial")) || bytes.Contains(body, []byte("message_stop")) {
		t.Fatalf("SSE changed: %d %s", resp.StatusCode, body)
	}
	sc, err := r.Scenario("wire")
	if err != nil {
		t.Fatal(err)
	}
	if len(sc.MockResponses) != 1 {
		t.Fatal("matrix ignored protocol restriction")
	}
}
func TestInvalidDefinitions(t *testing.T) {
	for _, source := range []string{
		"[]", "id: x\nkind: mystery", "id: x\nkind: mock\nmock: {}\nresponses: {}",
		"id: x\nkind: mock\nmock: {id: other}", "id: x\nkind: mock\nmock: {delay: -1ms}", "id: x\nkind: mock\nmock: {typo: 1}",
		"id: x\nkind: sequence\nsequence: {steps: [{status: 201}]}", "id: x\nkind: decision\ndecision: {strategy: random}", "id: x\nkind: decision\ndecision: {strategy: first, confidence: 2}",
		"id: x\nkind: recording\nresponses: {unknown: {body: {}}}", "id: x\nkind: mock\nmock: {}\nprotocols: [unknown]",
		"- {id: x, kind: mock, mock: {}}\n- {id: x, kind: mock, mock: {}}", "id: x\nkind: mock\nmock: {}\ncontent: conflicting",
		"id: x\nkind: mock\nmock: {tool_call: {name: a}, tool_calls: [{name: b}]}",
	} {
		if _, err := spec.Decode([]byte(source)); err == nil {
			t.Fatalf("accepted invalid definition: %s", source)
		}
	}
	for _, source := range []string{"id: x\nkind: mock\nmock: {}\nprotocols: [google]", "id: x\nkind: transform\ntransform: {builtin: missing}", "id: x\nkind: builtin\nbuiltin: missing"} {
		defs, err := spec.Decode([]byte(source))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := spec.New(defs); err == nil {
			t.Fatalf("compiled invalid definition: %s", source)
		}
	}
}

func TestRawSSEBytesAndArgumentsPrecision(t *testing.T) {
	raw := ": comment\r\nid: event-1\r\nevent: delta\r\ndata: first\r\ndata: second\r\n\r\n"
	d := spec.Definition{ID: "raw", Kind: spec.Recording, Responses: map[scenario.ResponseFormat]scenario.Response{scenario.FormatOpenAIChat: {RawStream: &raw}}}
	r, err := spec.New([]spec.Definition{d})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(r)
	defer server.Close()
	resp, err := server.Client().Post(server.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"raw","stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != raw {
		t.Fatalf("raw stream changed: %q", body)
	}
	r = compile(t, `id: numbers
kind: mock
mock:
  tool_calls:
    - name: lookup
      arguments: {id: 9007199254740993}
`)
	_, body = call(t, r, "/v1/chat/completions", `{"model":"numbers","messages":[]}`)
	args := gjson.GetBytes(body, "choices.0.message.tool_calls.0.function.arguments").String()
	if !strings.Contains(args, "9007199254740993") {
		t.Fatalf("integer precision lost: %s", args)
	}
}

func TestNativeExpectations(t *testing.T) {
	r := compile(t, `id: decide
kind: decision
decision: {strategy: first}
expect:
  http_status: 200
  json_fields:
    answers.pick.choice: a
`)
	status, body := call(t, r, "/v1/decisions", `{"model":"decide","state":"test","questions":{"pick":{"type":"choice","criteria":{"a":"A","b":"B"}}}}`)
	result := &check.RoundTripResult{HTTPStatus: status, RawBody: body}
	if err := r.Check("decide", result); err != nil {
		t.Fatal(err)
	}
	result.RawBody = []byte(`{"answers":{"pick":{"choice":"b"}}}`)
	if err := r.Check("decide", result); err == nil {
		t.Fatal("native mismatch passed")
	}
}

func TestExplicitToolStreamingContentIsRetained(t *testing.T) {
	cfg := vmodel.SharedMockSpec{ID: "stream-tool", StreamChunks: []string{"stream-only"}, ToolCalls: []vmodel.ToolCallConfig{{Name: "run", Arguments: map[string]any{"message": "display"}}}}
	exported, err := spec.Export([]spec.Definition{spec.FromMock(cfg)}, "yaml")
	if err != nil {
		t.Fatal(err)
	}
	defs, err := spec.Decode(exported)
	if err != nil {
		t.Fatal(err)
	}
	if defs[0].Mock.Content != "" || len(defs[0].Mock.StreamChunks) == 0 || len(defs[0].Mock.ToolCalls) != 1 {
		t.Fatal("explicit stream content was lost")
	}
	r, err := spec.New(defs)
	if err != nil {
		t.Fatal(err)
	}
	sc, err := r.Scenario("stream-tool")
	if err != nil {
		t.Fatal(err)
	}
	status, body := sc.MockResponses[scenario.FormatOpenAIChat].NonStream()
	if status != 200 || gjson.GetBytes(body, "choices.0.message.content").String() != "" {
		t.Fatalf("non-stream behavior changed: %s", body)
	}
	var streamText strings.Builder
	for _, line := range sc.MockResponses[scenario.FormatOpenAIChat].Stream() {
		if strings.HasPrefix(line, "data:") {
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			streamText.WriteString(gjson.Get(payload, "choices.0.delta.content").String())
		}
	}
	if streamText.String() != "stream-only" {
		t.Fatalf("stream behavior changed: %q", streamText.String())
	}
}

func TestBuiltinExpectationFields(t *testing.T) {
	r := compile(t, `id: configured
kind: mock
mock: {content: hello}
expect:
  http_status: 200
  role: assistant
  tool_call_count: 1
  tool_calls:
    - name: get_weather
      arguments: {location: Paris}
  usage_non_zero: true
`)
	result := &check.RoundTripResult{HTTPStatus: 200, Role: "assistant", ToolCalls: []check.ToolCallResult{{Name: "get_weather", Arguments: `{"location" : "Paris"}`}}, Usage: &check.TokenUsage{InputTokens: 1}}
	if err := r.Check("configured", result); err != nil {
		t.Fatal(err)
	}
	result.ToolCalls[0].Arguments = `{"nested":{"location":"Paris"}}`
	if err := r.Check("configured", result); err == nil {
		t.Fatal("nested argument matched top-level expectation")
	}
}
