package afk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/afk/msgparam"
)

// recordingLog captures what a Harness checkpoints, and when.
type recordingLog struct {
	mu       sync.Mutex
	appended []anthropic.BetaMessageParam
	batches  [][]anthropic.BetaMessageParam
	failWith error
}

func (l *recordingLog) Append(msgs ...anthropic.BetaMessageParam) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.failWith != nil {
		return l.failWith
	}
	l.appended = append(l.appended, msgs...)
	l.batches = append(l.batches, msgs)
	return nil
}

func (l *recordingLog) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.appended)
}

func (l *recordingLog) roles() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, 0, len(l.appended))
	for _, m := range l.appended {
		out = append(out, msgparam.BetaRole(m))
	}
	return out
}

func lastRole(msgs []anthropic.BetaMessageParam) string {
	return msgparam.BetaRole(msgs[len(msgs)-1])
}

// TestHarness_CheckpointsEachStep is the durability property: a multi-step run
// writes as it goes rather than only at the end, so an interruption keeps the
// work already done.
func TestHarness_CheckpointsEachStep(t *testing.T) {
	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&reqCount, 1) == 1 {
			writeSSE(t, w, toolUseResponse("toolu_1", "get_weather", `{"city":"Paris"}`))
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

	assert.Equal(t, 2, res.Steps)
	assert.Equal(t, len(res.Messages), log.count(), "everything the run produced should be logged")

	// Two checkpoints, one per step — not a single flush at the end.
	require.Len(t, log.batches, 2, "each completed step should checkpoint")
	assert.Len(t, log.batches[0], 3, "first batch: user prompt, assistant tool_use, tool results")
	assert.Len(t, log.batches[1], 1, "second batch: the final assistant turn")
}

// TestHarness_KeepsWorkWhenStepFails is what the checkpoint is for: the first
// step ran tools whose side effects already happened, and a failure on the
// second must not throw that away.
func TestHarness_KeepsWorkWhenStepFails(t *testing.T) {
	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&reqCount, 1) == 1 {
			writeSSE(t, w, toolUseResponse("toolu_1", "get_weather", `{"city":"Paris"}`))
			return
		}
		http.Error(w, "upstream exploded", http.StatusInternalServerError)
	}))
	defer srv.Close()

	tool := &fakeTool{name: "get_weather", result: "72F"}
	eng, err := NewEngine(Config{BaseURL: srv.URL, APIKey: "k", Model: "m", Tools: []Tool{tool}})
	require.NoError(t, err)

	log := &recordingLog{}
	res, err := NewHarness(eng, log).Run(context.Background(), nil, "weather?", &recordingSink{})
	require.Error(t, err, "the failing model call should surface")
	assert.True(t, res.Interrupted)

	assert.Equal(t, int32(1), atomic.LoadInt32(&tool.called), "the tool really did run")
	assert.GreaterOrEqual(t, log.count(), 3,
		"the completed step must be durable even though the run then failed")
}

// TestHarness_InterruptedRunLeavesReplayableTranscript pins the invariant a
// partial run has to satisfy: it may not end on a user turn. Appending the next
// user message to a transcript that already ends on one puts two in a row,
// which the API rejects — so persisting partial runs would otherwise wedge the
// conversation permanently.
func TestHarness_InterruptedRunLeavesReplayableTranscript(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(t, w, toolUseResponse("toolu_loop", "loop_tool", `{"city":"x"}`))
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{
		BaseURL: srv.URL, APIKey: "k", Model: "m",
		MaxIterations: 2,
		Tools:         []Tool{&fakeTool{name: "loop_tool", result: "still going"}},
	})
	require.NoError(t, err)

	log := &recordingLog{}
	res, err := NewHarness(eng, log).Run(context.Background(), nil, "loop", &recordingSink{})
	require.NoError(t, err)
	assert.True(t, res.Interrupted)

	// The run ended on tool results, which are a user turn; the harness closes
	// it out so the transcript alternates.
	assert.Equal(t, "assistant", lastRole(res.Messages),
		"an interrupted transcript must not end on a user turn")
	assert.Equal(t, "assistant", lastRole(log.appended),
		"and what was persisted must satisfy the same invariant")
	assert.Equal(t, len(res.Messages), log.count())
}

// A model call that fails on the very first step leaves only the user prompt,
// which is a user turn — the same invariant applies.
func TestHarness_FirstStepFailureStillLeavesReplayableTranscript(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	require.NoError(t, err)

	log := &recordingLog{}
	res, err := NewHarness(eng, log).Run(context.Background(), nil, "hello?", &recordingSink{})
	require.Error(t, err)

	require.NotEmpty(t, res.Messages)
	assert.Equal(t, "assistant", lastRole(res.Messages))
	assert.Equal(t, []string{"user", "assistant"}, log.roles(), "the prompt is kept, closed out so the next turn can be appended")
}

// A run that ends normally already ends on an assistant turn; no note is added.
func TestHarness_CompletedRunGetsNoInterruptionNote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(t, w, textResponse("all done"))
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	require.NoError(t, err)

	log := &recordingLog{}
	res, err := NewHarness(eng, log).Run(context.Background(), nil, "hi", &recordingSink{})
	require.NoError(t, err)
	assert.False(t, res.Interrupted)
	assert.Equal(t, "all done", res.FinalText)
	require.Len(t, res.Messages, 2)
	assert.Equal(t, "all done", msgparam.BetaContent(res.Messages[1])[0].OfText.Text,
		"a completed run should not be annotated")
}

// Cancellation mid-run is the /stop path: the work so far is kept and the
// transcript is closed out.
func TestHarness_CancelledRunIsCheckpointedAndClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&reqCount, 1) == 1 {
			writeSSE(t, w, toolUseResponse("toolu_1", "get_weather", `{"city":"Paris"}`))
			return
		}
		writeSSE(t, w, textResponse("should not get here"))
	}))
	defer srv.Close()

	// Cancel once the first step's tool has run, so the second step never starts.
	tool := &cancellingTool{name: "get_weather", result: "72F", cancel: cancel}
	eng, err := NewEngine(Config{BaseURL: srv.URL, APIKey: "k", Model: "m", Tools: []Tool{tool}})
	require.NoError(t, err)

	log := &recordingLog{}
	res, err := NewHarness(eng, log).Run(ctx, nil, "weather?", &recordingSink{})
	require.ErrorIs(t, err, context.Canceled)
	assert.True(t, res.Interrupted)

	assert.Equal(t, "assistant", lastRole(res.Messages))
	assert.Equal(t, len(res.Messages), log.count(), "cancelled work is still durable")
	assert.Equal(t, int32(1), atomic.LoadInt32(&reqCount), "the next step never started")
}

// A log that cannot be written must not fail the run: the tool side effects
// already happened, and dropping the turn would lose them without undoing them.
func TestHarness_LogFailureDoesNotFailTheRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(t, w, textResponse("fine"))
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	require.NoError(t, err)

	log := &recordingLog{failWith: errors.New("disk full")}
	res, err := NewHarness(eng, log).Run(context.Background(), nil, "hi", &recordingSink{})
	require.NoError(t, err, "a persistence failure is not a run failure")
	assert.Equal(t, "fine", res.FinalText)
}

// A nil log means "no durability wanted", which is how Engine.Run behaves.
func TestHarness_NilLogRunsNormally(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(t, w, textResponse("no log needed"))
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	require.NoError(t, err)

	res, err := NewHarness(eng, nil).Run(context.Background(), nil, "hi", &recordingSink{})
	require.NoError(t, err)
	assert.Equal(t, "no log needed", res.FinalText)
	assert.Len(t, res.Messages, 2)
}

// cancellingTool cancels the run's context from inside a tool call, which is
// where a real /stop lands: after the model has committed to tools.
type cancellingTool struct {
	name   string
	result string
	cancel context.CancelFunc
}

func (c *cancellingTool) Param() anthropic.BetaToolParam {
	return anthropic.BetaToolParam{
		Name:        c.name,
		Description: anthropic.String("cancels the run"),
		InputSchema: anthropic.BetaToolInputSchemaParam{
			Properties: map[string]any{"city": map[string]any{"type": "string"}},
		},
	}
}

func (c *cancellingTool) Call(ctx context.Context, raw json.RawMessage) (string, error) {
	c.cancel()
	return c.result, nil
}

// steeringTool steers the harness from inside a tool call, which is exactly
// when a real follow-up arrives: the model has committed to tools and the user
// is watching it work.
type steeringTool struct {
	name    string
	result  string
	harness func() *Harness
	texts   []string
	fired   int32
}

func (s *steeringTool) Param() anthropic.BetaToolParam {
	return anthropic.BetaToolParam{
		Name:        s.name,
		Description: anthropic.String("steers mid-run"),
		InputSchema: anthropic.BetaToolInputSchemaParam{
			Properties: map[string]any{"city": map[string]any{"type": "string"}},
		},
	}
}

func (s *steeringTool) Call(ctx context.Context, raw json.RawMessage) (string, error) {
	if atomic.AddInt32(&s.fired, 1) == 1 {
		for _, txt := range s.texts {
			s.harness().Steer(txt)
		}
	}
	return s.result, nil
}

// lastUserText concatenates the text blocks of the last user message in a
// request body captured from the wire.
func lastUserText(t *testing.T, body map[string]any) string {
	t.Helper()
	msgs, _ := body["messages"].([]any)
	require.NotEmpty(t, msgs)
	last, _ := msgs[len(msgs)-1].(map[string]any)
	blocks, _ := last["content"].([]any)
	var out string
	for _, b := range blocks {
		bm, _ := b.(map[string]any)
		if bm["type"] == "text" {
			s, _ := bm["text"].(string)
			out += s
		}
	}
	return out
}

// TestHarness_SteeringReachesTheModelOnTheNextStep is the point of the feature:
// a message sent while the agent is working is delivered on the next model
// call, instead of being rejected as a concurrent request.
func TestHarness_SteeringReachesTheModelOnTheNextStep(t *testing.T) {
	srv := &capturingServer{respond: func(n int) string {
		if n == 1 {
			return toolUseResponse("toolu_1", "get_weather", `{"city":"Paris"}`)
		}
		return textResponse("switched to Lyon")
	}}
	ts := httptest.NewServer(srv.handler(t))
	defer ts.Close()

	var h *Harness
	tool := &steeringTool{
		name: "get_weather", result: "72F",
		harness: func() *Harness { return h },
		texts:   []string{"actually, make it Lyon"},
	}
	eng, err := NewEngine(Config{BaseURL: ts.URL, APIKey: "k", Model: "m", Tools: []Tool{tool}})
	require.NoError(t, err)
	h = NewHarness(eng, nil)

	res, err := h.Run(context.Background(), nil, "weather in Paris?", &recordingSink{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Steered)

	// The second request carries the steering text.
	require.Len(t, srv.bodies, 2)
	assert.Contains(t, lastUserText(t, srv.body(1)), "actually, make it Lyon",
		"the follow-up must reach the model on the next step")
}

// After a tool step the transcript already ends on a user turn carrying tool
// results, so steering has to join that message rather than start a second user
// turn beside it — consecutive user messages are rejected by the API.
func TestHarness_SteeringMergesIntoToolResultTurn(t *testing.T) {
	srv := &capturingServer{respond: func(n int) string {
		if n == 1 {
			return toolUseResponse("toolu_1", "get_weather", `{"city":"Paris"}`)
		}
		return textResponse("ok")
	}}
	ts := httptest.NewServer(srv.handler(t))
	defer ts.Close()

	var h *Harness
	tool := &steeringTool{
		name: "get_weather", result: "72F",
		harness: func() *Harness { return h },
		texts:   []string{"and check the humidity"},
	}
	eng, err := NewEngine(Config{BaseURL: ts.URL, APIKey: "k", Model: "m", Tools: []Tool{tool}})
	require.NoError(t, err)
	h = NewHarness(eng, nil)

	res, err := h.Run(context.Background(), nil, "weather?", &recordingSink{})
	require.NoError(t, err)

	// user, assistant(tool_use), user(tool_result + steering), assistant
	require.Len(t, res.Messages, 4)
	merged := res.Messages[2]
	assert.Equal(t, "user", msgparam.BetaRole(merged))
	mergedContent := msgparam.BetaContent(merged)
	require.Len(t, mergedContent, 2, "steering joins the tool_result message")
	assert.NotNil(t, mergedContent[0].OfToolResult)
	require.NotNil(t, mergedContent[1].OfText)
	assert.Equal(t, "and check the humidity", mergedContent[1].OfText.Text)

	// Roles alternate: no two user messages in a row anywhere.
	for i := 1; i < len(res.Messages); i++ {
		assert.NotEqual(t, msgparam.BetaRole(res.Messages[i-1]), msgparam.BetaRole(res.Messages[i]),
			"roles must alternate at index %d", i)
	}
}

// Steering that lands after a text-only step is a follow-up: the model was
// about to stop, and the run must continue instead of stranding the message
// until the user types again.
func TestHarness_SteeringAfterTextStepContinuesTheRun(t *testing.T) {
	var reqCount int32
	steered := make(chan struct{})

	srv := &capturingServer{respond: func(n int) string {
		return textResponse("reply " + string(rune('0'+n)))
	}}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqCount, 1)
		srv.handler(t)(w, r)
	}))
	defer ts.Close()

	eng, err := NewEngine(Config{BaseURL: ts.URL, APIKey: "k", Model: "m"})
	require.NoError(t, err)
	h := NewHarness(eng, nil)

	// Steer from a sink callback, which fires while the first step is still in
	// flight — the model produced text and the run is about to end.
	sink := &steeringSink{onText: func() {
		select {
		case <-steered:
		default:
			h.Steer("one more thing")
			close(steered)
		}
	}}

	res, err := h.Run(context.Background(), nil, "hi", sink)
	require.NoError(t, err)

	assert.Equal(t, 1, res.Steered)
	assert.Equal(t, 2, res.Steps, "the run continues rather than stopping on the text step")
	assert.Equal(t, int32(2), atomic.LoadInt32(&reqCount))

	// user, assistant, user(steering), assistant
	require.Len(t, res.Messages, 4)
	assert.Equal(t, "user", msgparam.BetaRole(res.Messages[2]))
	assert.Equal(t, "one more thing", msgparam.BetaContent(res.Messages[2])[0].OfText.Text)
}

// Steering is durable: what the model was shown is what lands in the log.
func TestHarness_SteeringIsCheckpointed(t *testing.T) {
	srv := &capturingServer{respond: func(n int) string {
		if n == 1 {
			return toolUseResponse("toolu_1", "get_weather", `{"city":"Paris"}`)
		}
		return textResponse("done")
	}}
	ts := httptest.NewServer(srv.handler(t))
	defer ts.Close()

	var h *Harness
	tool := &steeringTool{
		name: "get_weather", result: "72F",
		harness: func() *Harness { return h },
		texts:   []string{"remember this"},
	}
	eng, err := NewEngine(Config{BaseURL: ts.URL, APIKey: "k", Model: "m", Tools: []Tool{tool}})
	require.NoError(t, err)
	log := &recordingLog{}
	h = NewHarness(eng, log)

	res, err := h.Run(context.Background(), nil, "weather?", &recordingSink{})
	require.NoError(t, err)

	require.Equal(t, len(res.Messages), log.count(),
		"the log must match what the model was shown, steering included")
	var found bool
	for _, m := range log.appended {
		for _, b := range msgparam.BetaContent(m) {
			if b.OfText != nil && b.OfText.Text == "remember this" {
				found = true
			}
		}
	}
	assert.True(t, found, "steering text must be persisted, not just sent")
}

// Steering outside a run is refused so the caller starts a normal turn instead
// of dropping the user's message on the floor.
func TestHarness_SteerOutsideRunIsRefused(t *testing.T) {
	eng, err := NewEngine(Config{BaseURL: "http://unused", APIKey: "k", Model: "m"})
	require.NoError(t, err)
	h := NewHarness(eng, nil)

	assert.False(t, h.Steer("nobody home"), "no run means nothing to steer")
	assert.False(t, h.Steer(""), "an empty message is not steering")
}

// A run that ends must not leave queued text behind to be replayed into the
// next turn, where it would answer a question nobody is asking any more.
func TestHarness_QueuedSteeringIsDroppedWhenTheRunEnds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(t, w, textResponse("done"))
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	require.NoError(t, err)
	h := NewHarness(eng, nil)

	_, err = h.Run(context.Background(), nil, "hi", &recordingSink{})
	require.NoError(t, err)

	assert.False(t, h.Steer("too late"), "the run is over; this should start a new turn")
	assert.Empty(t, h.drainSteering())
}

// steeringSink runs a callback when the model produces text.
type steeringSink struct {
	recordingSink
	onText func()
}

func (s *steeringSink) OnText(delta string) {
	s.recordingSink.OnText(delta)
	if s.onText != nil {
		s.onText()
	}
}
