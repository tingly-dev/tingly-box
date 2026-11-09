package afk

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturedRequest is what the server saw: the decoded body plus the headers we
// care about. Server-side compaction is configured across both — the flag is a
// header, the edit is in the body — so a test that only checks one proves
// nothing.
type capturedRequest struct {
	body    map[string]any
	betas   string
	rawBody []byte
}

type headerCapturingServer struct {
	reqs    []capturedRequest
	respond func(n int) string
}

func (c *headerCapturingServer) handler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var body map[string]any
		require.NoError(t, json.Unmarshal(raw, &body))
		c.reqs = append(c.reqs, capturedRequest{
			body:    body,
			betas:   r.Header.Get("anthropic-beta"),
			rawBody: raw,
		})
		writeSSE(t, w, c.respond(len(c.reqs)))
	}
}

// compactEdit digs the compaction edit out of a captured request body.
func compactEdit(t *testing.T, req capturedRequest) map[string]any {
	t.Helper()
	cm, ok := req.body["context_management"].(map[string]any)
	if !ok {
		return nil
	}
	edits, ok := cm["edits"].([]any)
	require.True(t, ok, "context_management should carry an edits list")
	for _, e := range edits {
		em, _ := e.(map[string]any)
		if em["type"] == "compact_20260112" {
			return em
		}
	}
	return nil
}

// TestEngine_SendsServerCompactionByDefault is the default-on behavior: a plain
// engine asks the API to compact for it, on both halves of the contract.
func TestEngine_SendsServerCompactionByDefault(t *testing.T) {
	srv := &headerCapturingServer{respond: func(int) string { return textResponse("hi") }}
	ts := httptest.NewServer(srv.handler(t))
	defer ts.Close()

	eng, err := NewEngine(Config{BaseURL: ts.URL, APIKey: "k", Model: "m"})
	require.NoError(t, err)

	_, _, err = eng.Run(context.Background(), nil, "hello", &recordingSink{})
	require.NoError(t, err)

	require.Len(t, srv.reqs, 1)
	req := srv.reqs[0]

	assert.Contains(t, req.betas, betaCompaction,
		"the beta flag travels as a header; without it the body edit is not honored")

	edit := compactEdit(t, req)
	require.NotNil(t, edit, "the request should carry a compact_20260112 edit")

	trigger, ok := edit["trigger"].(map[string]any)
	require.True(t, ok, "the edit should carry an explicit trigger")
	assert.Equal(t, "input_tokens", trigger["type"])
	assert.Equal(t, float64(DefaultServerCompactTrigger), trigger["value"],
		"the trigger is pinned here rather than inherited from the API default")

	assert.NotEmpty(t, edit["instructions"], "summaries should be steered toward agent state")
	assert.NotContains(t, string(req.rawBody), "pause_after_compaction",
		"compaction should be invisible to the run, not a step to resume")
}

func TestEngine_ServerCompactionTriggerIsConfigurable(t *testing.T) {
	srv := &headerCapturingServer{respond: func(int) string { return textResponse("hi") }}
	ts := httptest.NewServer(srv.handler(t))
	defer ts.Close()

	eng, err := NewEngine(Config{
		BaseURL: ts.URL, APIKey: "k", Model: "m",
		ServerCompactTrigger: 42_000,
	})
	require.NoError(t, err)

	_, _, err = eng.Run(context.Background(), nil, "hello", &recordingSink{})
	require.NoError(t, err)

	edit := compactEdit(t, srv.reqs[0])
	require.NotNil(t, edit)
	trigger, _ := edit["trigger"].(map[string]any)
	assert.Equal(t, float64(42_000), trigger["value"])
}

// The escape hatch has to actually remove both halves, not just one.
func TestEngine_ServerCompactionCanBeDisabled(t *testing.T) {
	srv := &headerCapturingServer{respond: func(int) string { return textResponse("hi") }}
	ts := httptest.NewServer(srv.handler(t))
	defer ts.Close()

	eng, err := NewEngine(Config{
		BaseURL: ts.URL, APIKey: "k", Model: "m",
		DisableServerCompaction: true,
	})
	require.NoError(t, err)

	_, _, err = eng.Run(context.Background(), nil, "hello", &recordingSink{})
	require.NoError(t, err)

	req := srv.reqs[0]
	assert.NotContains(t, req.betas, betaCompaction)
	assert.Nil(t, compactEdit(t, req))
	assert.NotContains(t, string(req.rawBody), "context_management")
}

// TestHarness_ReportsServerCompaction covers the read-back: a compaction block
// in the response is the API saying it compacted, and the run reports it.
func TestHarness_ReportsServerCompaction(t *testing.T) {
	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&reqCount, 1) == 1 {
			writeSSE(t, w, compactedToolUseResponse("toolu_1", "get_weather", `{"city":"Paris"}`))
			return
		}
		writeSSE(t, w, textResponse("72F"))
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{
		BaseURL: srv.URL, APIKey: "k", Model: "m",
		Tools: []Tool{&fakeTool{name: "get_weather", result: "72F"}},
	})
	require.NoError(t, err)

	log := &recordingLog{}
	res, err := NewHarness(eng, log).Run(context.Background(), nil, "weather?", &recordingSink{})
	require.NoError(t, err)

	assert.Equal(t, 1, res.Compactions, "the compaction block should be noticed")
	assert.Equal(t, "72F", res.FinalText, "and the run carries on normally")
}

// A run the API did not compact reports nothing — the counter is a fact read off
// the response, not a guess from token arithmetic.
func TestHarness_ReportsNoCompactionWhenNoneHappened(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(t, w, textResponse("short"))
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	require.NoError(t, err)

	res, err := NewHarness(eng, nil).Run(context.Background(), nil, "hi", &recordingSink{})
	require.NoError(t, err)
	assert.Zero(t, res.Compactions)
}

// TestHarness_CompactionBlockSurvivesHistoryAndPersistence is now load-bearing.
// The API replaces the compacted history using the compaction block we send
// back, so a block dropped from history — in memory or through the session log's
// JSON round trip — means the next request re-sends the full conversation the
// compaction was supposed to shrink.
func TestHarness_CompactionBlockSurvivesHistoryAndPersistence(t *testing.T) {
	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&reqCount, 1) == 1 {
			writeSSE(t, w, compactedToolUseResponse("toolu_1", "get_weather", `{"city":"Paris"}`))
			return
		}
		writeSSE(t, w, textResponse("done"))
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{
		BaseURL: srv.URL, APIKey: "k", Model: "m",
		Tools: []Tool{&fakeTool{name: "get_weather", result: "72F"}},
	})
	require.NoError(t, err)

	log := &recordingLog{}
	res, err := NewHarness(eng, log).Run(context.Background(), nil, "weather?", &recordingSink{})
	require.NoError(t, err)

	assert.True(t, hasCompactionBlock(res.Messages), "the block must stay in the live conversation")
	assert.True(t, hasCompactionBlock(log.appended), "and in what was persisted")

	// Round-trip the persisted history the way the session store does.
	data, err := json.Marshal(log.appended)
	require.NoError(t, err)
	var restored []anthropic.BetaMessageParam
	require.NoError(t, json.Unmarshal(data, &restored))
	assert.True(t, hasCompactionBlock(restored),
		"a compaction block that does not survive persistence un-does the compaction")
	assert.Contains(t, string(data), "the earlier conversation was summarized",
		"the summary itself has to be preserved, not just the block type")
}

func hasCompactionBlock(msgs []anthropic.BetaMessageParam) bool {
	for _, m := range msgs {
		for _, b := range m.Content {
			if b.OfCompaction != nil {
				return true
			}
		}
	}
	return false
}

// compactedToolUseResponse is toolUseResponse preceded by a compaction block,
// which is what the API returns on a request it compacted.
func compactedToolUseResponse(id, name, inputJSON string) string {
	w := &sseWriter{}
	w.event("message_start", `{"type":"message_start","message":{"id":"msg_c","type":"message","role":"assistant","model":"test","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":90000,"output_tokens":1}}}`)
	w.event("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"compaction","content":""}}`)
	w.event("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"compaction_delta","content":"the earlier conversation was summarized"}}`)
	w.event("content_block_stop", `{"type":"content_block_stop","index":0}`)
	w.event("content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"`+id+`","name":"`+name+`","input":{}}}`)
	pj, _ := json.Marshal(inputJSON)
	w.event("content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":`+string(pj)+`}}`)
	w.event("content_block_stop", `{"type":"content_block_stop","index":1}`)
	w.event("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":5}}`)
	w.event("message_stop", `{"type":"message_stop"}`)
	return w.String()
}
