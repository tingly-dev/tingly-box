package recording_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/recording/recordingtest"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// The recorder emits exactly once per request: RecordError only annotates,
// RecordResponse emits terminal success (idempotent), and FinalizeIfPending
// is the orchestrator backstop that flushes failed/aborted requests. These
// tests pin that lifecycle — the request-recording guarantee this branch
// hardens (.design/recording.md).

func newLifecycleRecorder(t *testing.T) (rec interface {
	RecordError(error)
	RecordResponse(provider *typ.Provider, model string)
	FinalizeIfPending()
}, mem *recordingtest.MemExporter, flush func()) {
	t.Helper()
	const scenario = typ.RuleScenario("test")
	h, sink, m := recordingtest.NewRecordingTestHandler(t, scenario, obs.RecordModeStagedRequestResponse)
	c, _ := recordingtest.NewRecordingTestContext(t, []byte(`{"model":"m"}`))
	r := h.EnsureProtocolRecorder(c, string(scenario), &typ.Provider{Name: "p"}, "m", obs.RecordModeStagedRequestResponse, nil)
	require.NotNil(t, r)
	return r, m, func() { require.NoError(t, sink.ForceFlush(recordingtest.CtxWithTimeout(t))) }
}

// A noted error alone must not emit; finalize flushes it once, with the error.
func TestRecorderLifecycle_ErrorNotedThenFinalized(t *testing.T) {
	rec, mem, flush := newLifecycleRecorder(t)

	rec.RecordError(errors.New("attempt failed"))
	flush()
	require.Empty(t, mem.Snapshot(), "RecordError must not emit")

	rec.FinalizeIfPending()
	rec.FinalizeIfPending() // idempotent
	flush()
	records := mem.Snapshot()
	require.Len(t, records, 1, "finalize must emit exactly once")
	assert.Equal(t, "attempt failed", records[0].Err)
}

// A failed attempt followed by a successful one (failover) must produce one
// clean success record — not a premature error record, not an empty duplicate.
func TestRecorderLifecycle_FailedAttemptThenSuccess(t *testing.T) {
	rec, mem, flush := newLifecycleRecorder(t)

	rec.RecordError(errors.New("attempt 1 transform failed"))
	rec.RecordResponse(&typ.Provider{Name: "p2"}, "m")
	rec.FinalizeIfPending() // orchestrator defer — must be a no-op after success
	flush()

	records := mem.Snapshot()
	require.Len(t, records, 1, "exactly one record per request")
	assert.Empty(t, records[0].Err, "terminal success must not carry the retried attempt's error")
	assert.Equal(t, "p2", records[0].Provider, "record must be bound to the winning attempt's provider")
}

// Duplicate success triggers (e.g. two dispatch paths both reporting) must
// not produce a second — previously empty, post-release — record.
func TestRecorderLifecycle_DuplicateResponseEmitsOnce(t *testing.T) {
	rec, mem, flush := newLifecycleRecorder(t)

	rec.RecordResponse(&typ.Provider{Name: "p"}, "m")
	rec.RecordResponse(&typ.Provider{Name: "p"}, "m")
	rec.FinalizeIfPending()
	flush()

	records := mem.Snapshot()
	require.Len(t, records, 1, "the emit latch must drop duplicate triggers")
	require.NotNil(t, records[0].OriginalRequest, "the single record must be the real one, not a post-release shell")
}

// A request that ends without any success or error report (aborted mid-way)
// still produces its record at finalize — request-side capture is never lost.
func TestRecorderLifecycle_FinalizeWithoutTerminalEvent(t *testing.T) {
	rec, mem, flush := newLifecycleRecorder(t)

	rec.FinalizeIfPending()
	flush()

	records := mem.Snapshot()
	require.Len(t, records, 1)
	assert.Empty(t, records[0].Err)
	require.NotNil(t, records[0].OriginalRequest, "request capture must survive an abort")
}
