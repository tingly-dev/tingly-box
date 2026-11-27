package protocoltest

// Functional phase of the duo environment: protocol correctness of one
// conversion route, driven end-to-end over real HTTP through both processes.
//
// The client-facing surface is always Anthropic (v1 or beta), so responses
// are parsed into the shared RoundTripResult shape and verified with the same
// check.Assertion vocabulary the matrix and replay tiers use — one DuoCheck
// per assertion. Only the assertion lists below are duo-specific.

import (
	"fmt"
	"io"
	"net/http"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
)

// DuoCheck is one functional verification result.
type DuoCheck struct {
	Route  string `json:"route"`
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail,omitempty"`
}

// duoStreamingAssertions verifies a streaming pass: full Anthropic SSE frame
// shape, assembled content, a stop reason, and usage propagated through the
// stream's terminal frames.
func duoStreamingAssertions() []Assertion {
	return []Assertion{
		AnthropicStreamShape(),
		AssertContentNonEmpty(),
		AssertFinishReasonNonEmpty(),
		AssertUsagePropagated(),
	}
}

// duoNonStreamingAssertions verifies a non-streaming pass: response body
// content and usage.
func duoNonStreamingAssertions() []Assertion {
	return []Assertion{
		AssertContentNonEmpty(),
		AssertUsagePropagated(),
	}
}

// RunFunctionalChecks verifies protocol correctness of one conversion route
// with a bodyBytes-sized conversation: streaming SSE shape, assembled
// content, usage propagation, and the non-streaming response body.
func (env *DuoEnv) RunFunctionalChecks(route DuoRoute, bodyBytes int) []DuoCheck {
	var checks []DuoCheck
	add := func(name string, pass bool, detail string) {
		checks = append(checks, DuoCheck{Route: route.Name, Name: name, Pass: pass, Detail: detail})
	}
	env.functionalPass(route, bodyBytes, true, add)
	env.functionalPass(route, bodyBytes, false, add)
	return checks
}

// functionalPass drives one request over the route, parses the response into
// the shared RoundTripResult shape, and runs the pass's assertion list.
func (env *DuoEnv) functionalPass(route DuoRoute, bodyBytes int, streaming bool, add func(name string, pass bool, detail string)) {
	prefix := "nonstream"
	assertions := duoNonStreamingAssertions()
	if streaming {
		prefix = "stream"
		assertions = duoStreamingAssertions()
	}

	resp, err := env.post(route, BuildConversationBody(route, bodyBytes, streaming))
	if err != nil {
		add(prefix+"/http", false, err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		add(prefix+"/http", false, fmt.Sprintf("status %d: %s", resp.StatusCode, b))
		return
	}
	add(prefix+"/http", true, "200")

	result := &RoundTripResult{IsStreaming: streaming, HTTPStatus: resp.StatusCode}
	if streaming {
		result.StreamEvents, result.RawBody = sse.ReadSSELines(resp.Body)
		fillFromParsedResult(result, assembleFromEvents(result.StreamEvents, protocol.APIStyleAnthropic))
	} else {
		raw, _ := io.ReadAll(resp.Body)
		result.RawBody = raw
		fillFromParsedResult(result, parseFromJSON(raw, protocol.APIStyleAnthropic))
	}

	for _, a := range assertions {
		if aerr := a.Check(result); aerr != nil {
			add(prefix+"/"+a.Name, false, aerr.Error())
		} else {
			add(prefix+"/"+a.Name, true, "")
		}
	}
}
