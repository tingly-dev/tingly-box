package main

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/tidwall/gjson"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocoltest"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark/scenario"
)

// The content_policy section: in-stream content-policy blocks from a Codex
// OAuth upstream, as a Responses client sees them (#2417, .design/codex.md
// §6).
//
// The ChatGPT backend answers 200 and then blocks inside the stream with an
// "error" event whose code names the policy. The Codex client reports the
// block as an upstream 400, so the gateway answers 400 before any output, and
// ends a stream that has already started with an "error" event and a
// response.failed, both carrying the code. Two controls check that another
// in-stream error keeps its handling.
//
// Each case routes through a Codex OAuth provider that points at the virtual
// upstream (SetupCodexAssemblyRoute): a plain OpenAI provider does not use the
// Codex client. The other client protocols, two accounts and the usage record
// are covered by the go tests in
// internal/protocoltest/content_policy_block_test.go.

const contentPolicyMessage = "This content was flagged for possible cybersecurity risk."

// contentPolicyCase is one row: the events the virtual Codex backend sends,
// whether the client streams, and what the client must receive.
type contentPolicyCase struct {
	name      string
	streaming bool
	events    []string
	check     func(r *protocoltest.RoundTripResult, upstreamCalls int) []string
}

func contentPolicyCases() []contentPolicyCase {
	return []contentPolicyCase{
		{name: "before_output/first_event", streaming: true,
			events: cpError("cyber_policy"),
			check:  cpBeforeOutput("cyber_policy")},
		{name: "before_output/non_streaming", streaming: false,
			events: slices.Concat(cpCreated(), cpText(), cpError("bio_policy")),
			check:  cpBeforeOutput("bio_policy")},
		{name: "mid_stream/after_created", streaming: true,
			events: slices.Concat(cpCreated(), cpError("misalignment_policy_violation")),
			check:  cpBlockTail("misalignment_policy_violation")},
		{name: "mid_stream/after_text", streaming: true,
			events: slices.Concat(cpCreated(), cpText(), cpError("cyber_policy")),
			check:  cpBlockTail("cyber_policy")},
		{name: "other_error/first_event", streaming: true,
			events: cpError("server_error"),
			check:  cpRetryable},
		{name: "other_error/mid_stream", streaming: true,
			events: slices.Concat(cpCreated(), cpText(), cpError("server_error")),
			check:  cpStreamFailed},
	}
}

// executeContentPolicy runs the content_policy section. Each case boots its
// own gateway and virtual upstream.
func executeContentPolicy(*protocoltest.Matrix) []protocoltest.TestResult {
	cases := contentPolicyCases()
	results := make([]protocoltest.TestResult, 0, len(cases))
	for _, c := range cases {
		results = append(results, runContentPolicyCase(c))
	}
	return results
}

func runContentPolicyCase(c contentPolicyCase) protocoltest.TestResult {
	start := time.Now()
	res := protocoltest.TestResult{
		Name:      "content_policy/" + c.name,
		Scenario:  c.name,
		Source:    protocol.TypeOpenAIResponses,
		Target:    protocol.TypeOpenAIResponses,
		Streaming: c.streaming,
	}
	failures, detail := c.run()
	for _, f := range failures {
		res.Errors = append(res.Errors, protocoltest.AssertionError{Assertion: "content_policy", Error: f, Context: detail})
	}
	res.Passed = len(failures) == 0
	res.Duration = time.Since(start)
	return res
}

func (c contentPolicyCase) run() (failures []string, detail string) {
	env, err := protocoltest.NewTestEnvForCLI()
	if err != nil {
		return []string{"start test env: " + err.Error()}, ""
	}
	defer env.Close()

	s := scenario.Scenario{
		Name: "content-policy-" + strings.ReplaceAll(c.name, "/", "-"),
		MockResponses: map[scenario.ResponseFormat]scenario.MockResponseBuilder{
			scenario.FormatOpenAIResponses: {Stream: func() []string { return c.events }},
		},
	}
	env.SetupCodexAssemblyRoute(protocol.TypeOpenAIResponses, s)
	before := env.VirtualCallCount()
	r, err := env.SendAsCLI(protocol.TypeOpenAIResponses, protocol.TypeOpenAIResponses, s, c.streaming)
	if err != nil {
		return []string{"send: " + err.Error()}, ""
	}
	return c.check(r, env.VirtualCallCount()-before), "client response:\n" + string(r.RawBody)
}

// ─── Upstream events ─────────────────────────────────────────────────────────

func cpEvent(name, data string) []string { return []string{"event: " + name, "data: " + data} }

// cpCreated starts the response "resp_x".
func cpCreated() []string {
	r := `{"id":"resp_x","object":"response","created_at":1700000000,"status":"in_progress","model":"m","output":[]}`
	return slices.Concat(
		cpEvent("response.created", `{"type":"response.created","sequence_number":0,"response":`+r+`}`),
		cpEvent("response.in_progress", `{"type":"response.in_progress","sequence_number":1,"response":`+r+`}`),
	)
}

// cpText starts a text answer.
func cpText() []string {
	return slices.Concat(
		cpEvent("response.output_item.added", `{"type":"response.output_item.added","sequence_number":2,"output_index":0,"item":{"id":"msg_x","type":"message","status":"in_progress","role":"assistant","content":[]}}`),
		cpEvent("response.content_part.added", `{"type":"response.content_part.added","sequence_number":3,"item_id":"msg_x","output_index":0,"content_index":0,"part":{"type":"output_text","text":"","annotations":[]}}`),
		cpEvent("response.output_text.delta", `{"type":"response.output_text.delta","sequence_number":4,"item_id":"msg_x","output_index":0,"content_index":0,"delta":"Sure, "}`),
	)
}

// cpError is an in-stream error event as the Codex backend sends it, with a
// nested error object.
func cpError(code string) []string {
	return cpEvent("error", fmt.Sprintf(`{"type":"error","sequence_number":9,"error":{"type":"invalid_request","code":%q,"message":%q,"param":null}}`, code, contentPolicyMessage))
}

// ─── Expectations ────────────────────────────────────────────────────────────

// cpBeforeOutput: nothing had reached the client, so it gets a 400 that names
// the code, from one upstream call.
func cpBeforeOutput(code string) func(*protocoltest.RoundTripResult, int) []string {
	return func(r *protocoltest.RoundTripResult, calls int) []string {
		var f []string
		if r.HTTPStatus != http.StatusBadRequest {
			f = append(f, fmt.Sprintf("status = %d, want 400", r.HTTPStatus))
		}
		if calls != 1 {
			f = append(f, fmt.Sprintf("upstream calls = %d, want 1", calls))
		}
		for _, want := range []string{"code=" + code, contentPolicyMessage} {
			if !strings.Contains(string(r.RawBody), want) {
				f = append(f, fmt.Sprintf("body does not contain %q", want))
			}
		}
		return f
	}
}

// cpBlockTail: the stream had started, so it ends with an "error" event and a
// response.failed, both carrying the code, and response.failed names the
// upstream response.
func cpBlockTail(code string) func(*protocoltest.RoundTripResult, int) []string {
	return func(r *protocoltest.RoundTripResult, calls int) []string {
		var f []string
		if r.HTTPStatus != http.StatusOK {
			f = append(f, fmt.Sprintf("status = %d, want 200", r.HTTPStatus))
		}
		if calls != 1 {
			f = append(f, fmt.Sprintf("upstream calls = %d, want 1", calls))
		}
		names, datas := cpClientEvents(r)
		n := len(names)
		if n < 2 || !slices.Equal(names[n-2:], []string{"error", "response.failed"}) {
			return append(f, fmt.Sprintf("events = %v, want the stream to end with [error response.failed]", names))
		}
		errEvent, failed := datas[n-2], datas[n-1]
		for _, c := range []struct{ what, got, want string }{
			{"error code", gjson.Get(errEvent, "code").String(), code},
			{"response.failed status", gjson.Get(failed, "response.status").String(), "failed"},
			{"response.failed error code", gjson.Get(failed, "response.error.code").String(), code},
			{"response.failed response id", gjson.Get(failed, "response.id").String(), "resp_x"},
		} {
			if c.got != c.want {
				f = append(f, fmt.Sprintf("%s = %q, want %q", c.what, c.got, c.want))
			}
		}
		return f
	}
}

// cpRetryable: another error before any output stays a retryable 5xx.
func cpRetryable(r *protocoltest.RoundTripResult, _ int) []string {
	if r.HTTPStatus < http.StatusInternalServerError {
		return []string{fmt.Sprintf("status = %d, want a retryable 5xx", r.HTTPStatus)}
	}
	return nil
}

// cpStreamFailed: another error after output still ends the stream with a
// stream_failed error event, and no response.failed is added.
func cpStreamFailed(r *protocoltest.RoundTripResult, _ int) []string {
	names, datas := cpClientEvents(r)
	n := len(names)
	if n == 0 || names[n-1] != "error" {
		return []string{fmt.Sprintf("events = %v, want the stream to end with an error event", names)}
	}
	var f []string
	if got := gjson.Get(datas[n-1], "code").String(); got != "stream_failed" {
		f = append(f, fmt.Sprintf("error code = %q, want %q", got, "stream_failed"))
	}
	if slices.Contains(names, "response.failed") {
		f = append(f, "response.failed was added for an error that is not a content-policy block")
	}
	return f
}

// cpClientEvents pairs each data line of the client stream with the event name
// before it, skipping a final [DONE].
func cpClientEvents(r *protocoltest.RoundTripResult) (names, datas []string) {
	name := ""
	for _, line := range r.StreamEvents {
		switch {
		case strings.HasPrefix(line, "event:"):
			name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data != "[DONE]" {
				names = append(names, name)
				datas = append(datas, data)
			}
			name = ""
		}
	}
	return names, datas
}
