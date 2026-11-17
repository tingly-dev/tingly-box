package spec_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/tingly-dev/tingly-box/vmodel/benchmark"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark/check"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark/scenario"
	"github.com/tingly-dev/tingly-box/vmodel/spec"
)

func TestRecordedFixtureFormatsAndReplay(t *testing.T) {
	yamlData := []byte(`id: recorded
kind: recording
responses:
  openai_chat:
    body:
      choices: [{message: {role: assistant, content: hello}}]
    stream:
      - 'data: {"choices":[{"delta":{"content":"hello"}}]}'
      - 'data: [DONE]'
expect:
  http_status: 200
  content_equals: hello
`)
	js := []byte(`{"id":"recorded","kind":"recording","responses":{"openai_chat":{"body":{"choices":[{"message":{"role":"assistant","content":"hello"}}]},"stream":["data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}","data: [DONE]"]}},"expect":{"http_status":200,"content_equals":"hello"}}`)
	a, err := decodeRecording(yamlData)
	if err != nil {
		t.Fatal(err)
	}
	b, err := decodeRecording(js)
	if err != nil {
		t.Fatal(err)
	}
	as, ab := a.MockResponses[scenario.FormatOpenAIChat].NonStream()
	bs, bb := b.MockResponses[scenario.FormatOpenAIChat].NonStream()
	if as != bs || !bytes.Equal(ab, bb) || !reflect.DeepEqual(a.MockResponses[scenario.FormatOpenAIChat].Stream(), b.MockResponses[scenario.FormatOpenAIChat].Stream()) {
		t.Fatal("JSON and YAML differ")
	}
	definitions, err := spec.Decode(js)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := spec.Export(definitions, "yaml")
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := decodeRecording(exported)
	if err != nil {
		t.Fatal(err)
	}
	_, reloadedBody := reloaded.MockResponses[scenario.FormatOpenAIChat].NonStream()
	if !bytes.Equal(ab, reloadedBody) {
		t.Fatalf("YAML export changed body: %s", reloadedBody)
	}
	server := benchmark.NewScenarioServer()
	server.RegisterScenario(a)
	for _, streaming := range []bool{false, true} {
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"recorded","stream":`+map[bool]string{false: "false", true: "true"}[streaming]+`}`))
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != 200 {
			t.Fatalf("status: %d: %s", response.Code, response.Body.String())
		}
		if streaming {
			if !bytes.Contains(response.Body.Bytes(), []byte("data: [DONE]")) {
				t.Fatal("lost stream terminator")
			}
		} else {
			var body struct {
				Choices []struct{ Message struct{ Content string } }
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			result := &check.RoundTripResult{HTTPStatus: response.Code, Content: body.Choices[0].Message.Content}
			for _, assertion := range a.Assertions {
				if err := assertion.Check(result); err != nil {
					t.Fatal(err)
				}
			}
			result.Content = "wrong"
			if a.Assertions[1].Check(result) == nil {
				t.Fatal("content mismatch passed")
			}
		}
	}
	// Response builders give each caller its own buffer.
	ab[0] = '!'
	_, again := a.MockResponses[scenario.FormatOpenAIChat].NonStream()
	if again[0] == '!' {
		t.Fatal("shared mutable body")
	}
}

func TestRecordedFixtureErrorsAndRawBody(t *testing.T) {
	s, err := decodeRecording([]byte(`id: error-recording
kind: recording
responses:
  anthropic:
    raw_body: '{ "error": {"message":"slow down"} }'
    stream_http_error: 429
`))
	if err != nil {
		t.Fatal(err)
	}
	if s.MockResponses[scenario.FormatAnthropic].Stream == nil {
		t.Fatal("pre-content HTTP errors must support streaming matrix runs")
	}
	server := benchmark.NewScenarioServer()
	server.RegisterScenario(s)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, httptest.NewRequest("POST", "/v1/messages", bytes.NewBufferString(`{"model":"error-recording","stream":true}`)))
	body, _ := io.ReadAll(w.Result().Body)
	if w.Code != 429 || string(body) != `{ "error": {"message":"slow down"} }` {
		t.Fatalf("error changed: %d %s", w.Code, body)
	}
	for _, input := range []string{
		"id: x\nkind: recording", "id: x\nkind: recording\nresponses:\n  unknown: {body: {}}",
		"id: x\nkind: recording\nresponses:\n  anthropic: {status: 600, body: {}}",
		"id: x\nkind: recording\nresponses:\n  anthropic: {body: {}, raw_body: text}",
		"id: x\nkind: recording\nresponses:\n  anthropic: {stream_http_error: 429, stream: []}",
		"id: x\nkind: recording\nresponses:\n  anthropic: {body: {}, typo: 1}",
		"id: x\nkind: recording\nresponses:\n  anthropic: {status: 500, stream_http_error: 429, body: {}}",
	} {
		if _, err := decodeRecording([]byte(input)); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}

func decodeRecording(data []byte) (scenario.Scenario, error) {
	definitions, err := spec.Decode(data)
	if err != nil {
		return scenario.Scenario{}, err
	}
	runtime, err := spec.New(definitions)
	if err != nil {
		return scenario.Scenario{}, err
	}
	return runtime.Scenario(definitions[0].ID)
}
