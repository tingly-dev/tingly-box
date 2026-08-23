package typ

import "testing"

func TestParseRecordingMode(t *testing.T) {
	cases := []struct {
		in   string
		want RecordingMode
	}{
		{"", RecordingModeDisabled},
		{"  ", RecordingModeDisabled},
		{"garbage", RecordingModeDisabled},
		// Legacy enum values expand to their point sets.
		{"request", "upstream_request"},
		{"request_response", "upstream_request,client_response"},
		{"staged_request_response", "client_request,upstream_request,client_response"},
		// Point sets normalize: dedupe, canonical pipeline order, unknown dropped.
		{"client_response,client_request", "client_request,client_response"},
		{"upstream_request,upstream_request", "upstream_request"},
		{"client_request, upstream_response ,nope", "client_request,upstream_response"},
		// Legacy value mixed with points unions.
		{"request,client_request", "client_request,upstream_request"},
	}
	for _, c := range cases {
		if got := ParseRecordingMode(c.in); got != c.want {
			t.Errorf("ParseRecordingMode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRecordingModeHas(t *testing.T) {
	m := RecordingMode("staged_request_response") // legacy value, un-normalized
	if !m.Has(RecordClientRequest) || !m.Has(RecordUpstreamRequest) || !m.Has(RecordClientResponse) {
		t.Errorf("legacy staged mode should select client_request+upstream_request+client_response, got %q", ParseRecordingMode(string(m)))
	}
	if m.Has(RecordUpstreamResponse) {
		t.Error("legacy staged mode must not select upstream_response")
	}
	if RecordingModeDisabled.Has(RecordClientRequest) {
		t.Error("disabled mode must select nothing")
	}
}

func TestIsValidRecordingMode(t *testing.T) {
	for _, ok := range []string{"", "request", "request_response", "staged_request_response",
		"client_request", "client_request,upstream_request,upstream_response,client_response",
		"request,client_response"} {
		if !IsValidRecordingMode(ok) {
			t.Errorf("IsValidRecordingMode(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"garbage", "client_request,typo", "all"} {
		if IsValidRecordingMode(bad) {
			t.Errorf("IsValidRecordingMode(%q) = true, want false", bad)
		}
	}
}

func TestEffectiveRecording(t *testing.T) {
	scenario := &ScenarioConfig{Flags: ScenarioFlags{RecordingV2: "request_response"}}
	ruleOff := &Rule{}
	ruleOn := &Rule{Flags: RuleFlags{Recording: "client_request"}}

	if got := EffectiveRecording(ruleOff, scenario); got != "upstream_request,client_response" {
		t.Errorf("scenario default should apply when rule unset, got %q", got)
	}
	if got := EffectiveRecording(ruleOn, scenario); got != "client_request" {
		t.Errorf("rule value should override scenario, got %q", got)
	}
	if got := EffectiveRecording(ruleOff, nil); got != RecordingModeDisabled {
		t.Errorf("no rule value, no scenario → disabled, got %q", got)
	}
	if got := EffectiveRecording(nil, scenario); got != "upstream_request,client_response" {
		t.Errorf("nil rule should fall back to scenario, got %q", got)
	}
}
