package afk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/afk/msgparam"
)

// sseWriter builds an Anthropic streaming SSE response. Each event is written
// as `event: <type>\n` followed by `data: <json>\n\n`, mirroring the sequence
// the SDK's ssestream decoder and Message.Accumulate expect.
type sseWriter struct {
	b strings.Builder
}

func (w *sseWriter) event(typ string, data string) {
	fmt.Fprintf(&w.b, "event: %s\ndata: %s\n\n", typ, data)
}

func (w *sseWriter) String() string { return w.b.String() }

// textResponse builds a complete SSE stream for a single assistant text message
// composed of the given fragments (streamed as separate text_delta events).
func textResponse(fragments ...string) string {
	w := &sseWriter{}
	w.event("message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"test","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}}`)
	w.event("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	for _, frag := range fragments {
		delta, _ := json.Marshal(frag)
		w.event("content_block_delta", fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%s}}`, delta))
	}
	w.event("content_block_stop", `{"type":"content_block_stop","index":0}`)
	w.event("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`)
	w.event("message_stop", `{"type":"message_stop"}`)
	return w.String()
}

// toolUseResponse builds a complete SSE stream for a single tool_use block with
// the given tool id, name, and JSON input (sent as one input_json_delta).
func toolUseResponse(id, name, inputJSON string) string {
	w := &sseWriter{}
	w.event("message_start", `{"type":"message_start","message":{"id":"msg_2","type":"message","role":"assistant","model":"test","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}}`)
	w.event("content_block_start", fmt.Sprintf(`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":%q,"name":%q,"input":{}}}`, id, name))
	pj, _ := json.Marshal(inputJSON)
	w.event("content_block_delta", fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":%s}}`, pj))
	w.event("content_block_stop", `{"type":"content_block_stop","index":0}`)
	w.event("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":5}}`)
	w.event("message_stop", `{"type":"message_stop"}`)
	return w.String()
}

func writeSSE(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(body))
	require.NoError(t, err)
}

// recordingSink captures all StreamSink callbacks for assertions.
type recordingSink struct {
	mu          sync.Mutex
	textFrags   []string
	toolCalls   []string
	toolResults []toolResult
	turnEnds    []turnEnd
	thinking    []string
}

type turnEnd struct {
	usage      anthropic.BetaUsage
	stopReason anthropic.BetaStopReason
}

type toolResult struct {
	name   string
	result string
	isErr  bool
}

func (s *recordingSink) OnText(delta string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.textFrags = append(s.textFrags, delta)
}

func (s *recordingSink) OnToolCall(name string, input json.RawMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolCalls = append(s.toolCalls, name)
}

func (s *recordingSink) OnToolResult(name, result string, isErr bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolResults = append(s.toolResults, toolResult{name: name, result: result, isErr: isErr})
}

func (s *recordingSink) OnThinking(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.thinking = append(s.thinking, text)
}

func (s *recordingSink) OnTurnEnd(usage anthropic.BetaUsage, stopReason anthropic.BetaStopReason) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turnEnds = append(s.turnEnds, turnEnd{usage: usage, stopReason: stopReason})
}

func (s *recordingSink) joinedText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.textFrags, "")
}

// fakeTool is a Tool whose Call returns a fixed string and records invocations.
type fakeTool struct {
	name     string
	result   string
	called   int32
	lastArgs json.RawMessage
}

func (f *fakeTool) Param() anthropic.BetaToolParam {
	return anthropic.BetaToolParam{
		Name:        f.name,
		Description: anthropic.String("a fake tool for testing"),
		InputSchema: anthropic.BetaToolInputSchemaParam{
			Properties: map[string]any{
				"city": map[string]any{"type": "string"},
			},
			Required: []string{"city"},
		},
	}
}

func (f *fakeTool) Call(ctx context.Context, rawInput json.RawMessage) (string, error) {
	atomic.AddInt32(&f.called, 1)
	f.lastArgs = rawInput
	return f.result, nil
}

func TestEngineRun_PlainText(t *testing.T) {
	frags := []string{"Hello", ", ", "world!"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(t, w, textResponse(frags...))
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{
		BaseURL: srv.URL,
		APIKey:  "dummy-key",
		Model:   "dummy-model",
	})
	require.NoError(t, err)

	sink := &recordingSink{}
	msgs, finalText, err := eng.Run(context.Background(), nil, "hi there", sink)
	require.NoError(t, err)

	assert.Equal(t, "Hello, world!", finalText)
	assert.Equal(t, "Hello, world!", sink.joinedText(), "aggregated text should equal final text")
	// Default (aggregated) mode: the whole turn arrives as a single OnText call.
	assert.Equal(t, []string{"Hello, world!"}, sink.textFrags, "aggregated mode should emit one OnText per turn")

	// user message + 1 assistant message
	require.Len(t, msgs, 2)
	assert.Equal(t, "user", msgparam.BetaRole(msgs[0]))
	assert.Equal(t, "assistant", msgparam.BetaRole(msgs[1]))
}

// TestEngineRun_PlainText_Streaming covers the opt-in StreamText mode where each
// text fragment is delivered as its own OnText call.
func TestEngineRun_PlainText_Streaming(t *testing.T) {
	frags := []string{"Hello", ", ", "world!"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(t, w, textResponse(frags...))
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{
		BaseURL:    srv.URL,
		APIKey:     "dummy-key",
		Model:      "dummy-model",
		StreamText: true,
	})
	require.NoError(t, err)

	sink := &recordingSink{}
	_, finalText, err := eng.Run(context.Background(), nil, "hi there", sink)
	require.NoError(t, err)

	assert.Equal(t, "Hello, world!", finalText)
	assert.Equal(t, "Hello, world!", sink.joinedText(), "concatenated stream fragments should equal final text")
	assert.Equal(t, frags, sink.textFrags, "streaming mode should emit each fragment as its own OnText call")
}

func TestEngineRun_ToolCall(t *testing.T) {
	tool := &fakeTool{name: "get_weather", result: "It is 72F and sunny."}

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&reqCount, 1)
		if n == 1 {
			writeSSE(t, w, toolUseResponse("toolu_abc", "get_weather", `{"city":"Paris"}`))
			return
		}
		writeSSE(t, w, textResponse("The weather ", "is nice."))
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{
		BaseURL: srv.URL,
		APIKey:  "dummy-key",
		Model:   "dummy-model",
		Tools:   []Tool{tool},
	})
	require.NoError(t, err)

	sink := &recordingSink{}
	msgs, finalText, err := eng.Run(context.Background(), nil, "what's the weather in Paris?", sink)
	require.NoError(t, err)

	// tool was invoked exactly once
	assert.Equal(t, int32(1), atomic.LoadInt32(&tool.called))
	assert.JSONEq(t, `{"city":"Paris"}`, string(tool.lastArgs))

	// sink callbacks fired
	assert.Equal(t, []string{"get_weather"}, sink.toolCalls)
	require.Len(t, sink.toolResults, 1)
	assert.Equal(t, "get_weather", sink.toolResults[0].name)
	assert.Equal(t, "It is 72F and sunny.", sink.toolResults[0].result)
	assert.False(t, sink.toolResults[0].isErr)

	// final text returned
	assert.Equal(t, "The weather is nice.", finalText)

	// messages: user, assistant(tool_use), user(tool_result), assistant(text)
	require.Len(t, msgs, 4)
	assert.Equal(t, "user", msgparam.BetaRole(msgs[0]))
	assert.Equal(t, "assistant", msgparam.BetaRole(msgs[1]))
	require.NotNil(t, msgparam.BetaContent(msgs[1])[0].OfToolUse, "second message should carry a tool_use block")
	assert.Equal(t, "user", msgparam.BetaRole(msgs[2]))
	require.NotNil(t, msgparam.BetaContent(msgs[2])[0].OfToolResult, "third message should carry a tool_result block")
	assert.Equal(t, "assistant", msgparam.BetaRole(msgs[3]))
	require.NotNil(t, msgparam.BetaContent(msgs[3])[0].OfText, "fourth message should carry a text block")
}

func TestEngineRun_MaxIterations(t *testing.T) {
	tool := &fakeTool{name: "loop_tool", result: "still going"}

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqCount, 1)
		// Always request a tool, never produce a final answer.
		writeSSE(t, w, toolUseResponse("toolu_loop", "loop_tool", `{"city":"x"}`))
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{
		BaseURL:       srv.URL,
		APIKey:        "dummy-key",
		Model:         "dummy-model",
		MaxIterations: 2,
		Tools:         []Tool{tool},
	})
	require.NoError(t, err)

	sink := &recordingSink{}
	_, finalText, err := eng.Run(context.Background(), nil, "loop forever", sink)
	require.NoError(t, err, "Run should return cleanly (with a warning) when max iterations is hit")

	// Exactly MaxIterations model calls were made — no infinite loop.
	assert.Equal(t, int32(2), atomic.LoadInt32(&reqCount))
	// Tool-only run: no assistant text was ever produced. This is the
	// "many tool calls, no message" scenario — finalText is empty and the sink
	// saw tool calls but no text. The loud WARN log lives in Run.
	assert.Empty(t, finalText, "a tool-only run produces no final text")
	assert.Empty(t, sink.textFrags, "no OnText should fire when the model only calls tools")
	assert.Equal(t, []string{"loop_tool", "loop_tool"}, sink.toolCalls)
}

func TestNewEngine_Validation(t *testing.T) {
	cases := map[string]Config{
		"missing BaseURL": {APIKey: "k", Model: "m"},
		"missing APIKey":  {BaseURL: "http://x", Model: "m"},
		"missing Model":   {BaseURL: "http://x", APIKey: "k"},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewEngine(cfg)
			assert.Error(t, err)
		})
	}

	_, err := NewEngine(Config{BaseURL: "http://x", APIKey: "k", Model: "m"})
	assert.NoError(t, err, "fully populated config should succeed")
}

// textThenStreamedToolResponse builds an SSE stream for an assistant turn that
// emits leading text AND a tool_use whose input arrives via input_json_delta
// (the real streaming shape, where the tool block's start carries empty input).
// This is the regression fixture for the "messages get lost" bug: reading text
// via AsAny().(TextBlock) or tool input via AsToolUse() yields empty values on a
// stream-accumulated message, because those reparse the block's original JSON
// rather than the delta-accumulated union fields.
func textThenStreamedToolResponse(text, toolID, toolName, inputJSON string) string {
	w := &sseWriter{}
	w.event("message_start", `{"type":"message_start","message":{"id":"msg_mix","type":"message","role":"assistant","model":"test","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}}`)
	// Text block (index 0): start + delta + stop.
	w.event("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	td, _ := json.Marshal(text)
	w.event("content_block_delta", fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%s}}`, td))
	w.event("content_block_stop", `{"type":"content_block_stop","index":0}`)
	// Tool block (index 1): start with empty input, then streamed input_json_delta.
	w.event("content_block_start", fmt.Sprintf(`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":%q,"name":%q,"input":{}}}`, toolID, toolName))
	pj, _ := json.Marshal(inputJSON)
	w.event("content_block_delta", fmt.Sprintf(`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":%s}}`, pj))
	w.event("content_block_stop", `{"type":"content_block_stop","index":1}`)
	w.event("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":5}}`)
	w.event("message_stop", `{"type":"message_stop"}`)
	return w.String()
}

// TestEngineRun_TextAndStreamedToolNotLost is the regression for messages being
// dropped when a turn carries both text and a streamed tool call. The text must
// reach the sink/finalText and the tool must receive the streamed input.
func TestEngineRun_TextAndStreamedToolNotLost(t *testing.T) {
	tool := &fakeTool{name: "lookup", result: "done"}

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&reqCount, 1) == 1 {
			writeSSE(t, w, textThenStreamedToolResponse("Let me look that up.", "toolu_1", "lookup", `{"city":"Paris"}`))
			return
		}
		writeSSE(t, w, textResponse("Here is the answer."))
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{BaseURL: srv.URL, APIKey: "k", Model: "m", Tools: []Tool{tool}})
	require.NoError(t, err)

	sink := &recordingSink{}
	_, finalText, err := eng.Run(context.Background(), nil, "look up Paris", sink)
	require.NoError(t, err)

	// Intermediate-turn text must not be lost.
	assert.Contains(t, sink.joinedText(), "Let me look that up.", "intermediate text was dropped")
	assert.Contains(t, sink.joinedText(), "Here is the answer.", "final text was dropped")
	assert.Equal(t, "Here is the answer.", finalText)
	// Streamed tool input must have reached the tool (not empty {}).
	assert.JSONEq(t, `{"city":"Paris"}`, string(tool.lastArgs), "streamed tool input was lost")
}

// usageTextResponse is textResponse with explicit cache accounting on
// message_start, so tests can assert the cache fields survive the SDK
// accumulator and reach the sink. Those fields are the only evidence that
// prompt-cache breakpoints are working, so they must not be dropped.
func usageTextResponse(inputTokens, cacheRead, cacheCreation int64, text string) string {
	w := &sseWriter{}
	w.event("message_start", fmt.Sprintf(`{"type":"message_start","message":{"id":"msg_u","type":"message","role":"assistant","model":"test","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":%d,"output_tokens":1,"cache_read_input_tokens":%d,"cache_creation_input_tokens":%d}}}`, inputTokens, cacheRead, cacheCreation))
	w.event("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	delta, _ := json.Marshal(text)
	w.event("content_block_delta", fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%s}}`, delta))
	w.event("content_block_stop", `{"type":"content_block_stop","index":0}`)
	w.event("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":7}}`)
	w.event("message_stop", `{"type":"message_stop"}`)
	return w.String()
}

// TestEngineRun_UsageReported checks that each assistant turn reports its token
// accounting and stop reason to the sink, including the cache counters.
func TestEngineRun_UsageReported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(t, w, usageTextResponse(100, 80, 20, "cached hello"))
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{BaseURL: srv.URL, APIKey: "dummy-key", Model: "dummy-model"})
	require.NoError(t, err)

	sink := &recordingSink{}
	_, _, err = eng.Run(context.Background(), nil, "hi", sink)
	require.NoError(t, err)

	require.Len(t, sink.turnEnds, 1)
	got := sink.turnEnds[0]
	assert.Equal(t, int64(100), got.usage.InputTokens)
	assert.Equal(t, int64(7), got.usage.OutputTokens, "message_delta usage should win for output tokens")
	assert.Equal(t, int64(80), got.usage.CacheReadInputTokens)
	assert.Equal(t, int64(20), got.usage.CacheCreationInputTokens)
	assert.Equal(t, anthropic.BetaStopReason("end_turn"), got.stopReason)
}

// TestEngineRun_UsageAccumulatesAcrossTurns checks that a tool round-trip
// reports usage once per assistant turn, and that Usage.Add sums them the way
// callers are expected to.
func TestEngineRun_UsageAccumulatesAcrossTurns(t *testing.T) {
	tool := &fakeTool{name: "get_weather", result: "72F"}

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&reqCount, 1) == 1 {
			writeSSE(t, w, toolUseResponse("toolu_1", "get_weather", `{"city":"Paris"}`))
			return
		}
		writeSSE(t, w, usageTextResponse(200, 150, 0, "nice out"))
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{BaseURL: srv.URL, APIKey: "dummy-key", Model: "dummy-model", Tools: []Tool{tool}})
	require.NoError(t, err)

	sink := &recordingSink{}
	_, _, err = eng.Run(context.Background(), nil, "weather?", sink)
	require.NoError(t, err)

	require.Len(t, sink.turnEnds, 2, "one OnTurnEnd per assistant turn")
	assert.Equal(t, anthropic.BetaStopReason("tool_use"), sink.turnEnds[0].stopReason)
	assert.Equal(t, anthropic.BetaStopReason("end_turn"), sink.turnEnds[1].stopReason)

	var total Usage
	for _, te := range sink.turnEnds {
		total.Add(te.usage)
	}
	assert.Equal(t, int64(201), total.InputTokens, "1 from the tool turn + 200 from the final turn")
	assert.Equal(t, int64(12), total.OutputTokens, "5 from the tool turn + 7 from the final turn")
	assert.Equal(t, int64(150), total.CacheReadInputTokens)
}

// capturingServer records the decoded request bodies the engine sends, so tests
// can assert on cache_control placement in the actual wire payload.
type capturingServer struct {
	mu       sync.Mutex
	bodies   []map[string]any
	respond  func(n int) string
	reqCount int
}

func (c *capturingServer) handler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var body map[string]any
		require.NoError(t, json.Unmarshal(raw, &body))

		c.mu.Lock()
		c.bodies = append(c.bodies, body)
		c.reqCount++
		n := c.reqCount
		c.mu.Unlock()

		writeSSE(t, w, c.respond(n))
	}
}

func (c *capturingServer) body(i int) map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bodies[i]
}

// lastBlockCacheControl returns the cache_control of the final content block of
// the final message in a captured request body, or nil when absent.
func lastBlockCacheControl(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	msgs, ok := body["messages"].([]any)
	require.True(t, ok, "request should carry messages")
	require.NotEmpty(t, msgs)
	last, ok := msgs[len(msgs)-1].(map[string]any)
	require.True(t, ok)
	blocks, ok := last["content"].([]any)
	require.True(t, ok, "last message content should be a block list")
	require.NotEmpty(t, blocks)
	tail, ok := blocks[len(blocks)-1].(map[string]any)
	require.True(t, ok)
	cc, _ := tail["cache_control"].(map[string]any)
	return cc
}

// TestEngineRun_CachesStaticPrefix checks the system prompt carries a cache
// breakpoint. Rendering order is tools -> system -> messages and a breakpoint
// covers everything up to its own block, so this one breakpoint caches the
// tools too — which is why the tools themselves carry none.
func TestEngineRun_CachesStaticPrefix(t *testing.T) {
	srv := &capturingServer{respond: func(int) string { return textResponse("hi") }}
	ts := httptest.NewServer(srv.handler(t))
	defer ts.Close()

	eng, err := NewEngine(Config{
		BaseURL: ts.URL, APIKey: "dummy-key", Model: "dummy-model",
		System: "you are a helpful guide",
		Tools:  []Tool{&fakeTool{name: "get_weather", result: "72F"}},
	})
	require.NoError(t, err)

	_, _, err = eng.Run(context.Background(), nil, "hi", &recordingSink{})
	require.NoError(t, err)

	body := srv.body(0)
	system, ok := body["system"].([]any)
	require.True(t, ok, "system should render as a block list")
	require.Len(t, system, 1)
	sysBlock := system[0].(map[string]any)
	cc, ok := sysBlock["cache_control"].(map[string]any)
	require.True(t, ok, "system block should carry a cache breakpoint")
	assert.Equal(t, "ephemeral", cc["type"])

	tools, ok := body["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 1)
	_, hasCC := tools[0].(map[string]any)["cache_control"]
	assert.False(t, hasCC, "system breakpoint already covers tools; marking them too would waste a breakpoint")
}

// TestEngineRun_CachesToolsWhenNoSystemPrompt covers the fallback: with no
// system prompt there is no block to carry the static-prefix breakpoint, so it
// moves to the last tool.
func TestEngineRun_CachesToolsWhenNoSystemPrompt(t *testing.T) {
	srv := &capturingServer{respond: func(int) string { return textResponse("hi") }}
	ts := httptest.NewServer(srv.handler(t))
	defer ts.Close()

	eng, err := NewEngine(Config{
		BaseURL: ts.URL, APIKey: "dummy-key", Model: "dummy-model",
		Tools: []Tool{
			&fakeTool{name: "first", result: "a"},
			&fakeTool{name: "second", result: "b"},
		},
	})
	require.NoError(t, err)

	_, _, err = eng.Run(context.Background(), nil, "hi", &recordingSink{})
	require.NoError(t, err)

	tools, ok := srv.body(0)["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 2)
	_, firstHasCC := tools[0].(map[string]any)["cache_control"]
	assert.False(t, firstHasCC, "only the final tool should close the cached prefix")
	cc, ok := tools[1].(map[string]any)["cache_control"].(map[string]any)
	require.True(t, ok, "last tool should carry the static-prefix breakpoint")
	assert.Equal(t, "ephemeral", cc["type"])
}

// TestEngineRun_RollingConversationBreakpoint checks the conversation
// breakpoint lands on the final block of every request and moves forward as the
// conversation grows — a pinned breakpoint would fall out of the lookback
// window on tool-heavy runs and stop being read.
func TestEngineRun_RollingConversationBreakpoint(t *testing.T) {
	srv := &capturingServer{respond: func(n int) string {
		if n == 1 {
			return toolUseResponse("toolu_1", "get_weather", `{"city":"Paris"}`)
		}
		return textResponse("nice out")
	}}
	ts := httptest.NewServer(srv.handler(t))
	defer ts.Close()

	eng, err := NewEngine(Config{
		BaseURL: ts.URL, APIKey: "dummy-key", Model: "dummy-model",
		System: "guide", Tools: []Tool{&fakeTool{name: "get_weather", result: "72F"}},
	})
	require.NoError(t, err)

	_, _, err = eng.Run(context.Background(), nil, "weather?", &recordingSink{})
	require.NoError(t, err)

	// Request 1 ends with the user's text block; request 2 ends with the
	// tool_result block. Both must carry the breakpoint.
	first := lastBlockCacheControl(t, srv.body(0))
	require.NotNil(t, first, "user text block should carry the rolling breakpoint")
	assert.Equal(t, "ephemeral", first["type"])

	second := lastBlockCacheControl(t, srv.body(1))
	require.NotNil(t, second, "tool_result block should carry the rolling breakpoint")
	assert.Equal(t, "ephemeral", second["type"])

	// The breakpoint moved rather than accumulating: exactly one block in the
	// second request carries cache_control.
	msgs := srv.body(1)["messages"].([]any)
	marked := 0
	for _, m := range msgs {
		for _, b := range m.(map[string]any)["content"].([]any) {
			if _, ok := b.(map[string]any)["cache_control"]; ok {
				marked++
			}
		}
	}
	assert.Equal(t, 1, marked, "stale breakpoints must not accumulate across a run")
}

// TestEngineRun_CacheBreakpointNotPersistedToHistory is the load-bearing one:
// the returned history is persisted verbatim by Smart Guide and replayed on the
// next turn. If breakpoints were written into it in place they would accumulate
// across turns and eventually exceed the per-request limit, failing the request.
func TestEngineRun_CacheBreakpointNotPersistedToHistory(t *testing.T) {
	srv := &capturingServer{respond: func(n int) string {
		if n == 1 {
			return toolUseResponse("toolu_1", "get_weather", `{"city":"Paris"}`)
		}
		return textResponse("nice out")
	}}
	ts := httptest.NewServer(srv.handler(t))
	defer ts.Close()

	eng, err := NewEngine(Config{
		BaseURL: ts.URL, APIKey: "dummy-key", Model: "dummy-model",
		System: "guide", Tools: []Tool{&fakeTool{name: "get_weather", result: "72F"}},
	})
	require.NoError(t, err)

	msgs, _, err := eng.Run(context.Background(), nil, "weather?", &recordingSink{})
	require.NoError(t, err)

	// Round-trip the returned history the way the session store does, and check
	// no cache_control survived into it.
	data, err := json.Marshal(msgs)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "cache_control",
		"cache breakpoints belong to the request, not the persisted conversation")
}

// thinkingToolUseResponse builds an SSE stream for a turn that reasons and then
// calls a tool, producing no text block at all — the shape that used to publish
// the model's deliberation to the chat as if it were the reply.
func thinkingToolUseResponse(thinking, id, name, inputJSON string) string {
	w := &sseWriter{}
	w.event("message_start", `{"type":"message_start","message":{"id":"msg_t","type":"message","role":"assistant","model":"test","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}}`)
	w.event("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`)
	tj, _ := json.Marshal(thinking)
	w.event("content_block_delta", fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":%s}}`, tj))
	w.event("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-abc123"}}`)
	w.event("content_block_stop", `{"type":"content_block_stop","index":0}`)
	w.event("content_block_start", fmt.Sprintf(`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":%q,"name":%q,"input":{}}}`, id, name))
	pj, _ := json.Marshal(inputJSON)
	w.event("content_block_delta", fmt.Sprintf(`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":%s}}`, pj))
	w.event("content_block_stop", `{"type":"content_block_stop","index":1}`)
	w.event("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":5}}`)
	w.event("message_stop", `{"type":"message_stop"}`)
	return w.String()
}

// TestEngineRun_ThinkingIsNotPresentedAsAnswer is the regression guard: a turn
// that only reasons and calls a tool must not have its reasoning delivered as
// assistant text, and must not become the run's final answer.
func TestEngineRun_ThinkingIsNotPresentedAsAnswer(t *testing.T) {
	const reasoning = "The user probably means the Paris in France, let me check the weather there."

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&reqCount, 1) == 1 {
			writeSSE(t, w, thinkingToolUseResponse(reasoning, "toolu_1", "get_weather", `{"city":"Paris"}`))
			return
		}
		writeSSE(t, w, textResponse("It is 72F in Paris."))
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{
		BaseURL: srv.URL, APIKey: "dummy-key", Model: "dummy-model",
		Tools: []Tool{&fakeTool{name: "get_weather", result: "72F"}},
	})
	require.NoError(t, err)

	sink := &recordingSink{}
	_, finalText, err := eng.Run(context.Background(), nil, "weather in Paris?", sink)
	require.NoError(t, err)

	assert.Equal(t, "It is 72F in Paris.", finalText, "the answer is the text block, never the reasoning")
	assert.NotContains(t, sink.joinedText(), reasoning, "reasoning must not be delivered as assistant text")
	assert.Equal(t, []string{"It is 72F in Paris."}, sink.textFrags)

	require.Equal(t, []string{reasoning}, sink.thinking, "reasoning belongs on its own channel")
}

// TestEngineRun_ThinkingBlocksSurviveHistoryRoundTrip covers replay: thinking
// blocks must go back to the API exactly as received, signature included. Smart
// Guide persists history as JSON between turns, so a block that does not
// survive that round trip is a rejected request on the user's next message.
func TestEngineRun_ThinkingBlocksSurviveHistoryRoundTrip(t *testing.T) {
	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&reqCount, 1) == 1 {
			writeSSE(t, w, thinkingToolUseResponse("deliberating", "toolu_1", "get_weather", `{"city":"Paris"}`))
			return
		}
		writeSSE(t, w, textResponse("done"))
	}))
	defer srv.Close()

	eng, err := NewEngine(Config{
		BaseURL: srv.URL, APIKey: "dummy-key", Model: "dummy-model",
		Tools: []Tool{&fakeTool{name: "get_weather", result: "72F"}},
	})
	require.NoError(t, err)

	msgs, _, err := eng.Run(context.Background(), nil, "weather?", &recordingSink{})
	require.NoError(t, err)

	// The assistant turn must retain the thinking block alongside the tool_use.
	assistant := msgs[1]
	var thinking *anthropic.BetaThinkingBlockParam
	for _, b := range msgparam.BetaContent(assistant) {
		if b.OfThinking != nil {
			thinking = b.OfThinking
		}
	}
	require.NotNil(t, thinking, "thinking block must stay in history, not be stripped")
	assert.Equal(t, "deliberating", thinking.Thinking)
	assert.Equal(t, "sig-abc123", thinking.Signature, "signature is what the API validates on replay")

	// Round-trip through the session store's encoding.
	data, err := json.Marshal(msgs)
	require.NoError(t, err)
	var restored []anthropic.BetaMessageParam
	require.NoError(t, json.Unmarshal(data, &restored))

	var restoredThinking *anthropic.BetaThinkingBlockParam
	for _, b := range msgparam.BetaContent(restored[1]) {
		if b.OfThinking != nil {
			restoredThinking = b.OfThinking
		}
	}
	require.NotNil(t, restoredThinking, "thinking block must survive persistence")
	assert.Equal(t, "deliberating", restoredThinking.Thinking)
	assert.Equal(t, "sig-abc123", restoredThinking.Signature,
		"a dropped signature makes the next turn a 400")
}

// TestEngine_CapsToolOutputThatToolDidNotCap is the backstop: the context limit
// is an invariant of the run loop, not a convention each tool has to remember.
// A tool that returns unbounded output must not be able to swamp the prompt.
func TestEngine_CapsToolOutputThatToolDidNotCap(t *testing.T) {
	var reqCount int32
	var second []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&reqCount, 1) == 1 {
			writeSSE(t, w, toolUseResponse("toolu_1", "gusher", `{}`))
			return
		}
		second, _ = io.ReadAll(r.Body)
		writeSSE(t, w, textResponse("ok"))
	}))
	defer srv.Close()

	huge := strings.Repeat("x*y*z*", 400_000) // ~2.4MB, far past DefaultMaxOutputBytes
	eng, err := NewEngine(Config{
		BaseURL: srv.URL, APIKey: "k", Model: "m",
		Tools: []Tool{&fakeTool{name: "gusher", result: huge}},
	})
	require.NoError(t, err)

	_, _, err = eng.Run(context.Background(), nil, "go", &recordingSink{})
	require.NoError(t, err)

	require.NotNil(t, second, "the follow-up request should carry the tool result")
	assert.Less(t, len(second), 4*DefaultMaxOutputBytes,
		"an uncapped tool result must not reach the model at full size")
	assert.Contains(t, string(second), "Output truncated",
		"and the model must be told it is looking at a partial result")
}
