package typ

import "strings"

// RecordingPoint identifies one capture point along the gateway pipeline.
// A recording selection is a set of these points (comma-separated in storage),
// replacing the old three-value mode enum — the old modes were just fixed
// combinations of these points (see legacyRecordingModes).
//
//	client_request    — inbound request as the client sent it (pre-transform)
//	upstream_request  — outbound request dispatched to the provider (post-transform)
//	upstream_response — provider's raw response (wire level; capture lands with the wire recorder, .design/recording.md Phase 3)
//	client_response   — response as returned to the client
type RecordingPoint string

const (
	RecordClientRequest    RecordingPoint = "client_request"
	RecordUpstreamRequest  RecordingPoint = "upstream_request"
	RecordUpstreamResponse RecordingPoint = "upstream_response"
	RecordClientResponse   RecordingPoint = "client_response"
)

// AllRecordingPoints lists the capture points in canonical (pipeline) order.
func AllRecordingPoints() []RecordingPoint {
	return []RecordingPoint{RecordClientRequest, RecordUpstreamRequest, RecordUpstreamResponse, RecordClientResponse}
}

// RecordingMode is a comma-separated set of RecordingPoints. "" means
// recording disabled. Legacy single-enum values (request / request_response /
// staged_request_response) are still accepted everywhere via
// ParseRecordingMode, so stored configs keep working without migration.
type RecordingMode string

const (
	RecordingModeDisabled RecordingMode = "" // Recording disabled (default)

	// Legacy enum values from the pre point-set model. Kept for stored-config
	// compat and the ParseRecordingMode mapping; new writes use point sets.
	RecordingModeRequestOnly           RecordingMode = "request"                 // → upstream_request
	RecordingModeRequestResponse       RecordingMode = "request_response"        // → upstream_request,client_response
	RecordingModeStagedRequestResponse RecordingMode = "staged_request_response" // → client_request,upstream_request,client_response
)

// legacyRecordingModes maps the pre point-set enum values onto point sets.
var legacyRecordingModes = map[string][]RecordingPoint{
	string(RecordingModeRequestOnly):           {RecordUpstreamRequest},
	string(RecordingModeRequestResponse):       {RecordUpstreamRequest, RecordClientResponse},
	string(RecordingModeStagedRequestResponse): {RecordClientRequest, RecordUpstreamRequest, RecordClientResponse},
}

func isKnownRecordingPoint(p RecordingPoint) bool {
	switch p {
	case RecordClientRequest, RecordUpstreamRequest, RecordUpstreamResponse, RecordClientResponse:
		return true
	}
	return false
}

// ParseRecordingMode normalizes raw into a canonical RecordingMode: legacy
// enum values expand to their point sets, points are deduped and put in
// canonical order, and unknown tokens are dropped. Returns
// RecordingModeDisabled when nothing valid remains.
func ParseRecordingMode(raw string) RecordingMode {
	seen := map[RecordingPoint]bool{}
	for tok := range strings.SplitSeq(raw, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if pts, ok := legacyRecordingModes[tok]; ok {
			for _, p := range pts {
				seen[p] = true
			}
			continue
		}
		if p := RecordingPoint(tok); isKnownRecordingPoint(p) {
			seen[p] = true
		}
	}
	if len(seen) == 0 {
		return RecordingModeDisabled
	}
	parts := make([]string, 0, len(seen))
	for _, p := range AllRecordingPoints() {
		if seen[p] {
			parts = append(parts, string(p))
		}
	}
	return RecordingMode(strings.Join(parts, ","))
}

// Has reports whether the mode (raw or normalized) selects the given point.
func (m RecordingMode) Has(p RecordingPoint) bool {
	if m == RecordingModeDisabled {
		return false
	}
	normalized := string(ParseRecordingMode(string(m)))
	return strings.Contains(","+normalized+",", ","+string(p)+",")
}

// Enabled reports whether the mode selects at least one capture point.
func (m RecordingMode) Enabled() bool {
	return ParseRecordingMode(string(m)) != RecordingModeDisabled
}

// IsValidRecordingMode reports whether mode is empty, or a comma-separated
// list whose every token is a known capture point or legacy enum value.
// Stricter than ParseRecordingMode (which silently drops unknown tokens):
// use this at config-write boundaries so typos are rejected, not swallowed.
func IsValidRecordingMode(mode string) bool {
	if strings.TrimSpace(mode) == "" {
		return true
	}
	for tok := range strings.SplitSeq(mode, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if _, ok := legacyRecordingModes[tok]; ok {
			continue
		}
		if !isKnownRecordingPoint(RecordingPoint(tok)) {
			return false
		}
	}
	return true
}

// EffectiveRecording resolves the recording selection for one request: the
// rule's recording flag overrides the scenario's recording_v2 default (rule
// non-empty wins — same "override" inheritance as thinking_effort). Both
// sides accept legacy values; the result is normalized.
func EffectiveRecording(rule *Rule, scenario *ScenarioConfig) RecordingMode {
	if rule != nil {
		if m := ParseRecordingMode(rule.Flags.Recording); m != RecordingModeDisabled {
			return m
		}
	}
	if scenario != nil {
		return ParseRecordingMode(string(scenario.Flags.RecordingV2))
	}
	return RecordingModeDisabled
}
