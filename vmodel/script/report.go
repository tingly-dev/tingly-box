package script

import (
	"fmt"
	"strings"
)

// Issue is one failure or warning of a run.
type Issue struct {
	Kind   string `json:"kind"`
	Track  string `json:"track,omitempty"`
	Step   string `json:"step,omitempty"`
	Seq    int    `json:"seq,omitempty"`
	Detail string `json:"detail"`
}

func (i Issue) String() string {
	where := i.Track
	if i.Step != "" {
		where += "." + i.Step
	}
	if where != "" {
		where = " " + where
	}
	seq := ""
	if i.Seq > 0 {
		seq = fmt.Sprintf(" (request %d)", i.Seq)
	}
	return fmt.Sprintf("%s%s%s: %s", i.Kind, where, seq, i.Detail)
}

// Issue kinds.
const (
	IssueUnmatched    = "unmatched"     // no track, step or background rule expected the request
	IssueAmbiguous    = "ambiguous"     // a new conversation matched several tracks
	IssueMismatch     = "mismatch"      // the next step's when did not hold
	IssueExhausted    = "exhausted"     // the track had no step left
	IssueExpect       = "expect"        // a step's expect failed
	IssueExpectResult = "expect_result" // a tool call's expect_result failed
	IssueTemplate     = "template"      // a {{var}} had no value
	IssueCapture      = "capture"       // a capture found nothing
	IssueAfterTimeout = "after_timeout" // a held step's after conditions never held
	IssueUnconsumed   = "unconsumed"    // a step was never served
	IssueResultMissed = "result_missed" // a call with expect_result never had its result seen
	IssueBackground   = "background"    // a background rule ran outside min/max
	IssueRetry        = "retry"         // a request repeated an answered one (warning)
	IssueFinished     = "finished"      // a request arrived after Finish
)

// Check is one assertion evaluated on a request.
type Check struct {
	On     string `json:"on"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// Entry is one request as the script saw it.
type Entry struct {
	Seq     int      `json:"seq"`
	Track   string   `json:"track,omitempty"`
	Step    string   `json:"step,omitempty"`
	Via     string   `json:"via"` // bind, continuation, retry, background, unmatched
	Class   string   `json:"class"`
	Model   string   `json:"model,omitempty"`
	Session string   `json:"session,omitempty"`
	HeldMS  int64    `json:"held_ms,omitempty"`
	Status  int      `json:"status"`
	Checks  []Check  `json:"checks,omitempty"`
	Text    string   `json:"text,omitempty"`
	Tools   []string `json:"tools,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// TrackReport is where one track ended.
type TrackReport struct {
	Name      string   `json:"name"`
	Bound     bool     `json:"bound"`
	Session   string   `json:"session,omitempty"`
	Served    []string `json:"served"`
	Remaining []string `json:"remaining"`
}

// Report is the outcome of a run so far (final after Finish).
type Report struct {
	Script     string         `json:"script"`
	Finished   bool           `json:"finished"`
	OK         bool           `json:"ok"`
	Requests   int            `json:"requests"`
	Failures   []Issue        `json:"failures"`
	Warnings   []Issue        `json:"warnings"`
	Tracks     []TrackReport  `json:"tracks"`
	Background map[string]int `json:"background"`
}

func (r Report) String() string {
	var b strings.Builder
	state := "running"
	if r.Finished {
		state = "finished"
	}
	verdict := "ok"
	if !r.OK {
		verdict = "FAILED"
	}
	fmt.Fprintf(&b, "script %s %s: %s, %d requests\n", r.Script, state, verdict, r.Requests)
	for _, f := range r.Failures {
		fmt.Fprintf(&b, "  FAIL %s\n", f)
	}
	for _, w := range r.Warnings {
		fmt.Fprintf(&b, "  warn %s\n", w)
	}
	for _, t := range r.Tracks {
		fmt.Fprintf(&b, "  track %s: served [%s]", t.Name, strings.Join(t.Served, " "))
		if len(t.Remaining) > 0 {
			fmt.Fprintf(&b, ", remaining [%s]", strings.Join(t.Remaining, " "))
		}
		if !t.Bound {
			b.WriteString(", never bound")
		}
		b.WriteString("\n")
	}
	return b.String()
}
