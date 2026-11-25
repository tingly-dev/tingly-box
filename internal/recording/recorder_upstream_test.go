package recording_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocolserver/recordingtest"
	"github.com/tingly-dev/tingly-box/internal/recording"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestUpstreamResponse_AttachedToFingerprint wires the wire-tap inputs through
// the recorder and asserts they land on fingerprint.upstream_response, with
// the same redaction as the rest of the fingerprint.
func TestUpstreamResponse_AttachedToFingerprint(t *testing.T) {
	const scenario = typ.RuleScenario("test")
	mode := obs.RecordMode(typ.RecordFinalResponse)
	h, sink, mem := recordingtest.NewRecordingTestHandler(t, scenario, mode)
	c, _ := recordingtest.NewRecordingTestContext(t, []byte(`{}`))
	provider := &typ.Provider{Name: "p", APIStyle: "anthropic"}

	recorder := h.EnsureProtocolRecorder(c, string(scenario), provider, "m", mode, nil)
	require.NotNil(t, recorder)
	require.Same(t, recorder, recording.RecorderFromContext(c.Request.Context()),
		"the recorder must ride the request context so the wire transport can find it")

	recorder.SetUpstreamResponse(http.StatusOK, http.Header{
		"Content-Type":                           {"application/json"},
		"X-Request-Id":                           {"req_0123456789"},
		"Anthropic-Ratelimit-Input-Tokens-Limit": {"40000"},
		"Set-Cookie":                             {"sid=secret"},
		"Authorization":                          {"Bearer secret"},
		"Openai-Organization":                    {"org-account"},
	})
	recorder.SetAssembledResponse(chatResponseBody())
	recorder.RecordResponse(provider, "m")

	require.NoError(t, sink.ForceFlush(recordingtest.CtxWithTimeout(t)))
	records := mem.Snapshot()
	require.Len(t, records, 1)
	require.NotNil(t, records[0].Fingerprint)

	up := records[0].Fingerprint.UpstreamResponse
	require.NotNil(t, up, "upstream_response must be attached when captured")
	assert.Equal(t, http.StatusOK, up.StatusCode)
	assert.Equal(t, "application/json", up.Headers["content-type"])
	assert.Equal(t, "req_*", up.Headers["x-request-id"])
	assert.Equal(t, "40000", up.Headers["anthropic-ratelimit-input-tokens-limit"])
	for _, dropped := range []string{"set-cookie", "authorization", "openai-organization"} {
		_, ok := up.Headers[dropped]
		assert.False(t, ok, "%s must not be persisted", dropped)
	}

	// The gateway's own fingerprint.headers stays separate from the upstream's.
	assert.NotContains(t, records[0].Fingerprint.Headers, "anthropic-ratelimit-input-tokens-limit")
}

// TestUpstreamResponse_LastAttemptWins mirrors the failover loop: a transport
// tap may fire once per attempt, and the emitted record should reflect the
// attempt that actually served the response.
func TestUpstreamResponse_LastAttemptWins(t *testing.T) {
	const scenario = typ.RuleScenario("test")
	mode := obs.RecordMode(typ.RecordFinalResponse)
	h, sink, mem := recordingtest.NewRecordingTestHandler(t, scenario, mode)
	c, _ := recordingtest.NewRecordingTestContext(t, []byte(`{}`))
	provider := &typ.Provider{Name: "p", APIStyle: "openai"}

	recorder := h.EnsureProtocolRecorder(c, string(scenario), provider, "m", mode, nil)
	require.NotNil(t, recorder)

	recorder.SetUpstreamResponse(http.StatusServiceUnavailable, http.Header{"Server": {"failed-attempt"}})
	recorder.SetUpstreamResponse(http.StatusOK, http.Header{"Server": {"served-attempt"}})
	recorder.SetAssembledResponse(chatResponseBody())
	recorder.RecordResponse(provider, "m")

	require.NoError(t, sink.ForceFlush(recordingtest.CtxWithTimeout(t)))
	records := mem.Snapshot()
	require.Len(t, records, 1)
	up := records[0].Fingerprint.UpstreamResponse
	require.NotNil(t, up)
	assert.Equal(t, http.StatusOK, up.StatusCode)
	assert.Equal(t, "served-attempt", up.Headers["server"])
}

// TestUpstreamResponse_IgnoredWithoutFinalResponse verifies the switch: when
// the final_response capture point is not selected, the wire tap's observation
// is dropped and no fingerprint (hence no upstream_response) is emitted.
func TestUpstreamResponse_IgnoredWithoutFinalResponse(t *testing.T) {
	const scenario = typ.RuleScenario("test")
	mode := obs.RecordMode(typ.RecordUpstreamRequest)
	h, sink, mem := recordingtest.NewRecordingTestHandler(t, scenario, mode)
	c, _ := recordingtest.NewRecordingTestContext(t, []byte(`{}`))
	provider := &typ.Provider{Name: "p", APIStyle: "openai"}

	recorder := h.EnsureProtocolRecorder(c, string(scenario), provider, "m", mode, nil)
	require.NotNil(t, recorder)

	recorder.SetUpstreamResponse(http.StatusOK, http.Header{"Content-Type": {"application/json"}})
	recorder.SetAssembledResponse(chatResponseBody())
	recorder.RecordResponse(provider, "m")

	require.NoError(t, sink.ForceFlush(recordingtest.CtxWithTimeout(t)))
	records := mem.Snapshot()
	require.Len(t, records, 1)
	assert.Nil(t, records[0].Fingerprint, "no fingerprint without the final_response point")
}
