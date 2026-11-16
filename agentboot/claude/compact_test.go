package claude

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/protocol"
)

// slashFixture is the stream of one real Claude Code 2.1.259 process: two
// ordinary turns, then /cost, /compact and /clear.
func slashFixture(t *testing.T) []protocol.Event {
	t.Helper()
	f, err := os.Open("../engine/testdata/claude-2.1.259-slash-commands.jsonl")
	require.NoError(t, err)
	defer f.Close()
	var out []protocol.Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var data map[string]any
		require.NoError(t, json.Unmarshal(sc.Bytes(), &data))
		ev := protocol.NewEventFromMap(data)
		ev.Raw = sc.Text()
		out = append(out, ev)
	}
	return out
}

func compactEvents(tr *Transport, evs []protocol.Event) []agentboot.CompactEvent {
	var out []agentboot.CompactEvent
	for _, ev := range evs {
		for _, m := range tr.AccumulateMessage(ev) {
			if ce, ok := m.(agentboot.CompactEvent); ok {
				out = append(out, ce)
			}
		}
	}
	return out
}

func TestTransport_ReportsARealCompaction(t *testing.T) {
	got := compactEvents(NewTransport(), slashFixture(t))
	require.Len(t, got, 2, "started, then done: the CLI's own success status adds nothing")
	assert.Equal(t, agentboot.CompactStarted, got[0].Phase)
	done := got[1]
	assert.Equal(t, agentboot.CompactDone, done.Phase)
	assert.Equal(t, agentboot.CompactManual, done.Result.Trigger)
	assert.Equal(t, int64(254), done.Result.TokensBefore)
	assert.Equal(t, int64(1024), done.Result.TokensAfter)
	assert.True(t, done.Result.Reported.Tokens)
	assert.Equal(t, int64(27), done.Result.DurationMS)
	assert.True(t, done.Result.Reported.Summary)
	assert.Contains(t, done.Result.Summary, "This session is being continued")
	assert.False(t, done.Result.Reported.Usage, "the CLI reports no tokens for the summary call")
	assert.Nil(t, done.Result.Usage)
}

func TestTransport_ReportsAFailedCompaction(t *testing.T) {
	tr := NewTransport()
	got := compactEvents(tr, []protocol.Event{
		{Type: "system", Data: map[string]any{"type": "system", "subtype": "status", "status": "compacting"}},
		{Type: "system", Data: map[string]any{"type": "system", "subtype": "status", "status": nil, "compact_result": "failed", "compact_error": "Not enough messages to compact."}},
	})
	require.Len(t, got, 2)
	assert.Equal(t, agentboot.CompactFailed, got[1].Phase)
	assert.Equal(t, "Not enough messages to compact.", got[1].Reason)
}

func TestTransport_AutoCompactionWithoutASummaryStillReports(t *testing.T) {
	tr := NewTransport()
	boundary := protocol.Event{Type: "system", Data: map[string]any{"type": "system", "subtype": "compact_boundary",
		"compact_metadata": map[string]any{"trigger": "auto", "pre_tokens": 900000.0, "post_tokens": 40000.0, "duration_ms": 5000.0}}}
	next := protocol.Event{Type: "assistant", Data: map[string]any{"type": "assistant"}, Raw: `{"type":"assistant"}`}
	got := compactEvents(tr, []protocol.Event{boundary, next})
	require.Len(t, got, 1, "the boundary is flushed by the next event")
	assert.Equal(t, agentboot.CompactAuto, got[0].Result.Trigger)
	assert.False(t, got[0].Result.Reported.Summary)
}

func TestAgent_CompactPrompt(t *testing.T) {
	a := &Agent{}
	assert.Equal(t, "/compact", a.CompactPrompt(""))
	assert.Equal(t, "/compact", a.CompactPrompt("  "))
	assert.Equal(t, "/compact keep the API notes", a.CompactPrompt(" keep the API notes "))
}

func TestTransport_ContextUsage(t *testing.T) {
	tr := NewTransport()
	assert.Equal(t, map[string]any{"subtype": "get_context_usage"},
		tr.EncodeControlRequest("c1", agentboot.ContextUsageRequest{}).(map[string]any)["request"])

	ev := protocol.Event{Type: "control_response", Data: map[string]any{"response": map[string]any{
		"subtype": "success", "request_id": "c1",
		"response": map[string]any{"totalTokens": 120.0, "maxTokens": 1000000.0, "autoCompactThreshold": 967000.0, "isAutoCompactEnabled": true}}}}
	got, err := tr.DecodeContextUsage(tr.ControlResponseBody(ev))
	require.NoError(t, err)
	assert.Equal(t, agentboot.ContextUsage{Used: 120, Max: 1000000, AutoCompactAt: 967000}, got)

	off, err := tr.DecodeContextUsage(map[string]any{"totalTokens": 5.0, "maxTokens": 10.0, "autoCompactThreshold": 9.0, "isAutoCompactEnabled": false})
	require.NoError(t, err)
	assert.Equal(t, int64(0), off.AutoCompactAt, "no automatic compaction when the CLI has it off")

	_, err = tr.DecodeContextUsage(map[string]any{})
	assert.Error(t, err)
}

var (
	_ agentboot.Compactor                  = (*Agent)(nil)
	_ agentboot.ContextUsageReader         = (*Transport)(nil)
	_ agentboot.ControlResponseBodyDecoder = (*Transport)(nil)
)
