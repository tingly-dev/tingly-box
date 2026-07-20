package protocoltest

// Codex passthrough phase of the duo environment (#1384): the client speaks
// OpenAI Responses to tb2's codex scenario surface — exactly what Codex CLI
// does — and tb2 passes through to tb1's /virtual responses endpoint.
//
// Three routes are wired (see duoCodexRules):
//
//	duo-e2e-codex-ok         → healthy stream, must end with response.completed
//	duo-e2e-codex-trunc-eof  → tb1 ends the body cleanly with no terminal
//	                           event; tb2 must convert the bare EOF into an
//	                           explicit error event (code upstream_truncated)
//	duo-e2e-codex-trunc-drop → tb1 hijacks and closes the TCP connection;
//	                           tb2's read error must surface as an error
//	                           event (code stream_failed)
//
// The truncation assertions pin the gateway's contract with strict clients:
// a Codex-side "stream closed before response.completed" with nothing else on
// the wire is a regression.

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

// duoCodexEvent is one parsed SSE frame from tb2's Responses surface.
type duoCodexEvent struct {
	Event string // "event:" line value (e.g. "response.completed", "error")
	Data  string // "data:" line payload
}

// postCodexResponses drives one streaming request through tb2's codex
// surface and parses the SSE body into frames.
func (env *DuoEnv) postCodexResponses(requestModel string) ([]duoCodexEvent, error) {
	body := fmt.Sprintf(`{"model":%q,"stream":true,"input":"say hello"}`, requestModel)
	req, err := http.NewRequest(http.MethodPost, env.TB2.BaseURL+"/tingly/codex/v1/responses", bytes.NewReader([]byte(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+env.TB2.ModelToken)
	resp, err := env.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b := make([]byte, 2048)
		n, _ := resp.Body.Read(b)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, b[:n])
	}

	var events []duoCodexEvent
	var cur duoCodexEvent
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event:"):
			cur.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			cur.Data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		case line == "":
			if cur.Event != "" || cur.Data != "" {
				events = append(events, cur)
				cur = duoCodexEvent{}
			}
		}
	}
	// A read error here is expected for the drop route only if tb2 failed to
	// convert it — tb2 itself always terminates the client stream properly,
	// so any scanner error is a finding, not noise.
	if err := sc.Err(); err != nil {
		return events, fmt.Errorf("client read error (gateway leaked the upstream break): %w", err)
	}
	return events, nil
}

// codexEventSummary compacts a frame list for check details.
func codexEventSummary(events []duoCodexEvent) string {
	var types []string
	for _, e := range events {
		t := e.Event
		if t == "" {
			t = gjson.Get(e.Data, "type").String()
		}
		if t == "" {
			t = "?"
		}
		types = append(types, t)
	}
	return strings.Join(types, ",")
}

// codexFindError returns the first error frame's code and message ("" if none).
func codexFindError(events []duoCodexEvent) (code, message string, found bool) {
	for _, e := range events {
		if e.Event != "error" && gjson.Get(e.Data, "type").String() != "error" {
			continue
		}
		return gjson.Get(e.Data, "error.code").String(), gjson.Get(e.Data, "error.message").String(), true
	}
	return "", "", false
}

// codexHasEvent reports whether a frame of the given event type is present.
func codexHasEvent(events []duoCodexEvent, typ string) bool {
	for _, e := range events {
		if e.Event == typ || gjson.Get(e.Data, "type").String() == typ {
			return true
		}
	}
	return false
}

// RunCodexPassthroughChecks drives the three codex passthrough routes and
// verifies the healthy stream completes and both truncation shapes surface as
// explicit in-band error events instead of a bare EOF.
func (env *DuoEnv) RunCodexPassthroughChecks() []DuoCheck {
	var checks []DuoCheck
	add := func(route, name string, pass bool, detail string) {
		checks = append(checks, DuoCheck{Route: route, Name: name, Pass: pass, Detail: detail})
	}

	// Healthy control: the passthrough must not misreport a completed stream.
	okEvents, err := env.postCodexResponses(DuoCodexOKModel)
	if err != nil {
		add("codex-ok", "http", false, err.Error())
	} else {
		add("codex-ok", "http", true, codexEventSummary(okEvents))
		add("codex-ok", "completed", codexHasEvent(okEvents, "response.completed"),
			"stream must end with response.completed")
		if _, msg, found := codexFindError(okEvents); found {
			add("codex-ok", "no-error-event", false, "unexpected error event: "+msg)
		} else {
			add("codex-ok", "no-error-event", true, "")
		}
	}

	// Truncation shapes: no terminal event may be fabricated, and the break
	// must reach the client as an explicit error event with the right code.
	for _, tc := range []struct {
		route, model, wantCode string
	}{
		{"codex-trunc-eof", DuoCodexTruncEOFModel, "upstream_truncated"},
		{"codex-trunc-drop", DuoCodexTruncDropModel, "stream_failed"},
	} {
		events, err := env.postCodexResponses(tc.model)
		if err != nil {
			add(tc.route, "http", false, err.Error())
			continue
		}
		add(tc.route, "http", true, codexEventSummary(events))
		add(tc.route, "no-fabricated-completed", !codexHasEvent(events, "response.completed"),
			"a truncated stream must not be reported as completed")
		add(tc.route, "delta-forwarded", codexHasEvent(events, "response.output_text.delta"),
			"pre-break deltas should pass through")
		code, msg, found := codexFindError(events)
		switch {
		case !found:
			add(tc.route, "error-event", false,
				"no error event on the wire — client sees a bare EOF (\"stream closed before response.completed\")")
		case code != tc.wantCode:
			add(tc.route, "error-event", false,
				fmt.Sprintf("error code %q (want %q): %s", code, tc.wantCode, msg))
		default:
			add(tc.route, "error-event", true, fmt.Sprintf("code=%s message=%q", code, msg))
		}
	}
	return checks
}
