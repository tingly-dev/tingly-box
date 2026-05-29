package afk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func (f *fakeTool) Name() string        { return f.name }
func (f *fakeTool) Description() string { return "a fake tool for testing" }
func (f *fakeTool) Schema() (map[string]any, []string) {
	return map[string]any{
		"city": map[string]any{"type": "string"},
	}, []string{"city"}
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
	assert.Equal(t, anthropic.MessageParamRoleUser, msgs[0].Role)
	assert.Equal(t, anthropic.MessageParamRoleAssistant, msgs[1].Role)
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
	assert.Equal(t, anthropic.MessageParamRoleUser, msgs[0].Role)
	assert.Equal(t, anthropic.MessageParamRoleAssistant, msgs[1].Role)
	require.NotNil(t, msgs[1].Content[0].OfToolUse, "second message should carry a tool_use block")
	assert.Equal(t, anthropic.MessageParamRoleUser, msgs[2].Role)
	require.NotNil(t, msgs[2].Content[0].OfToolResult, "third message should carry a tool_result block")
	assert.Equal(t, anthropic.MessageParamRoleAssistant, msgs[3].Role)
	require.NotNil(t, msgs[3].Content[0].OfText, "fourth message should carry a text block")
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
	_, _, err = eng.Run(context.Background(), nil, "loop forever", sink)
	require.NoError(t, err, "Run should return cleanly (with a warning) when max iterations is hit")

	// Exactly MaxIterations model calls were made — no infinite loop.
	assert.Equal(t, int32(2), atomic.LoadInt32(&reqCount))
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
