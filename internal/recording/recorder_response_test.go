package recording_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocolserver/recordingtest"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func chatResponseBody() map[string]interface{} {
	return map[string]interface{}{
		"id":                 "chatcmpl-AbC123",
		"model":              "gpt-4o-2024-08-06",
		"system_fingerprint": "fp_abc123",
		"service_tier":       "default",
		"choices": []interface{}{
			map[string]interface{}{"finish_reason": "stop"},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     float64(11),
			"completion_tokens": float64(4),
			"total_tokens":      float64(15),
		},
	}
}

// TestFinalResponse_OffByDefault verifies the switch: without the
// final_response point, no response side (and no fingerprint) is emitted even
// if the recorder captured one internally.
func TestFinalResponse_OffByDefault(t *testing.T) {
	const scenario = typ.RuleScenario("test")
	h, sink, mem := recordingtest.NewRecordingTestHandler(t, scenario, obs.RecordMode(typ.RecordUpstreamRequest))
	c, _ := recordingtest.NewRecordingTestContext(t, []byte(`{}`))

	recorder := h.EnsureProtocolRecorder(c, string(scenario), &typ.Provider{Name: "p"}, "m", obs.RecordMode(typ.RecordUpstreamRequest), nil)
	require.NotNil(t, recorder)
	recorder.SetAssembledResponse(chatResponseBody())
	recorder.RecordResponse(&typ.Provider{Name: "p"}, "m")

	require.NoError(t, sink.ForceFlush(recordingtest.CtxWithTimeout(t)))
	records := mem.Snapshot()
	require.Len(t, records, 1)
	assert.Nil(t, records[0].FinalResponse, "final_response must not be emitted unless selected")
	assert.Nil(t, records[0].Fingerprint, "fingerprint must not be emitted unless final_response is selected")
}

// TestFinalResponse_OnRedactedAndFingerprinted verifies the switch's on side:
// the body and a compact fingerprint are emitted, sensitive headers are
// dropped, and the fingerprint carries the identity/usage signals.
func TestFinalResponse_OnRedactedAndFingerprinted(t *testing.T) {
	const scenario = typ.RuleScenario("test")
	mode := obs.RecordMode(typ.RecordFinalResponse)
	h, sink, mem := recordingtest.NewRecordingTestHandler(t, scenario, mode)
	c, _ := recordingtest.NewRecordingTestContext(t, []byte(`{}`))

	// Sensitive + allowlisted headers set on the writer before assembly.
	c.Writer.Header().Set("Set-Cookie", "sid=secret")
	c.Writer.Header().Set("Authorization", "Bearer secret")
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.Header().Set("X-Request-Id", "req_0123456789abcdef")

	recorder := h.EnsureProtocolRecorder(c, string(scenario), &typ.Provider{Name: "p", APIStyle: "openai"}, "m", mode, nil)
	require.NotNil(t, recorder)
	recorder.SetAssembledResponse(chatResponseBody())
	recorder.RecordResponse(&typ.Provider{Name: "p", APIStyle: "openai"}, "m")

	require.NoError(t, sink.ForceFlush(recordingtest.CtxWithTimeout(t)))
	records := mem.Snapshot()
	require.Len(t, records, 1)
	rec := records[0]

	require.NotNil(t, rec.FinalResponse, "final_response must be emitted when selected")
	// Full headers keep debugging value but drop secrets.
	require.NotNil(t, rec.FinalResponse.Headers)
	assert.Equal(t, "application/json", rec.FinalResponse.Headers["Content-Type"])
	assert.NotContains(t, rec.FinalResponse.Headers, "Set-Cookie")
	assert.NotContains(t, rec.FinalResponse.Headers, "Authorization")

	require.NotNil(t, rec.Fingerprint)
	assert.Equal(t, "through_tb", rec.Fingerprint.Channel)
	assert.Equal(t, "chatcmpl-*", rec.Fingerprint.IDPattern, "raw ids are never persisted")
	assert.Equal(t, "fp_abc123", rec.Fingerprint.SystemFingerprint)
	assert.Equal(t, "application/json", rec.Fingerprint.Headers["content-type"])
	assert.Equal(t, "req_*", rec.Fingerprint.Headers["x-request-id"])
	assert.NotContains(t, rec.Fingerprint.Headers, "set-cookie")
	require.NotNil(t, rec.Fingerprint.Usage)
	assert.Equal(t, 11, rec.Fingerprint.Usage.InputTokens)
	assert.Equal(t, 4, rec.Fingerprint.Usage.OutputTokens)
	assert.Contains(t, rec.Fingerprint.FinishReasons, "stop")
	assert.GreaterOrEqual(t, rec.Fingerprint.ElapsedMs, int64(0))
}

// TestFinalResponse_ErrorPath verifies an errored request emits no response
// side even when final_response is selected.
func TestFinalResponse_ErrorPath(t *testing.T) {
	const scenario = typ.RuleScenario("test")
	mode := obs.RecordMode(typ.RecordFinalResponse)
	h, sink, mem := recordingtest.NewRecordingTestHandler(t, scenario, mode)
	c, _ := recordingtest.NewRecordingTestContext(t, []byte(`{}`))

	recorder := h.EnsureProtocolRecorder(c, string(scenario), &typ.Provider{Name: "p"}, "m", mode, nil)
	require.NotNil(t, recorder)
	recorder.RecordError(assert.AnError)

	require.NoError(t, sink.ForceFlush(recordingtest.CtxWithTimeout(t)))
	records := mem.Snapshot()
	require.Len(t, records, 1)
	assert.NotEmpty(t, records[0].Err)
	assert.Nil(t, records[0].FinalResponse)
	assert.Nil(t, records[0].Fingerprint)
}
