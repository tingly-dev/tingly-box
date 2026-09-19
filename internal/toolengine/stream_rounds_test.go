package toolengine

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"

	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ── Fixtures ────────────────────────────────────────────────────────────────
//
// A two-round Anthropic stream: the model asks for a server-executed tool,
// the interceptor answers it in-process, and the model then produces text.
// That is the shape of every tool-loop round trip, so the client-visible event
// sequence it yields is the contract worth pinning.

// A server-executed tool name. advisor is the built-in one every classifier
// already recognises, so the harness needs no registry setup.
const roundsToolName = "tingly_box_mcp__advisor__advisor"

func anthropicEvents(t *testing.T, raws ...string) []anthropic.BetaRawMessageStreamEventUnion {
	t.Helper()
	out := make([]anthropic.BetaRawMessageStreamEventUnion, 0, len(raws))
	for _, raw := range raws {
		var e anthropic.BetaRawMessageStreamEventUnion
		if err := json.Unmarshal([]byte(raw), &e); err != nil {
			t.Fatalf("unmarshal event %s: %v", raw, err)
		}
		out = append(out, e)
	}
	return out
}

func toolRoundEvents(t *testing.T) []anthropic.BetaRawMessageStreamEventUnion {
	return anthropicEvents(t,
		`{"type":"message_start","message":{"id":"m1","type":"message","role":"assistant","content":[],"model":"downstream","stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"`+roundsToolName+`","input":{}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"query\":\"go\"}"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":5}}`,
		`{"type":"message_stop"}`,
	)
}

func textRoundEvents(t *testing.T) []anthropic.BetaRawMessageStreamEventUnion {
	return anthropicEvents(t,
		`{"type":"message_start","message":{"id":"m2","type":"message","role":"assistant","content":[],"model":"downstream","stop_reason":null,"usage":{"input_tokens":20,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Done."}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":7}}`,
		`{"type":"message_stop"}`,
	)
}

// ── Doubles ─────────────────────────────────────────────────────────────────

type sliceStream struct {
	events []anthropic.BetaRawMessageStreamEventUnion
	idx    int
}

func (s *sliceStream) Next() bool   { s.idx++; return s.idx <= len(s.events) }
func (s *sliceStream) Current() any { return s.events[s.idx-1] }
func (s *sliceStream) Err() error   { return nil }
func (s *sliceStream) Close() error { return nil }

type roundsForwarder struct {
	rounds [][]anthropic.BetaRawMessageStreamEventUnion
	calls  int
}

func (f *roundsForwarder) ForwardStream(_ context.Context, _ any, _ string, _ any) (StreamHandle, error) {
	events := f.rounds[min(f.calls, len(f.rounds)-1)]
	f.calls++
	return &sliceStream{events: events}, nil
}

func (f *roundsForwarder) ForwardNonStream(context.Context, any, string, any) (any, error) {
	return nil, nil
}

// noopServerOps satisfies the usage-reporting dependency; the interceptor
// dereferences it unconditionally at the end of Run.
type noopServerOps struct{}

func (noopServerOps) TrackUsage(*gin.Context, int, int, int) {}
func (noopServerOps) CallMCPTool(context.Context, string, string, []map[string]any) (string, error) {
	return "", nil
}
func (noopServerOps) GetRecorder() ProtocolRecorder { return nil }

type cannedExecutor struct{ text string }

func (e cannedExecutor) ExecuteToolWithContext(ctx context.Context, tool Tool, _ []map[string]any) (context.Context, ToolExecutionResult, error) {
	return ctx, ToolExecutionResult{ToolUseID: tool.ID(), Contents: coretool.TextToolResult(e.text).Contents}, nil
}
func (e cannedExecutor) ExecuteTool(ctx context.Context, tool Tool, m []map[string]any) (ToolExecutionResult, error) {
	_, r, err := e.ExecuteToolWithContext(ctx, tool, m)
	return r, err
}
func (e cannedExecutor) ExecuteTools(ctx context.Context, tools []Tool, m []map[string]any) ([]ToolExecutionResult, error) {
	out := make([]ToolExecutionResult, 0, len(tools))
	for _, tool := range tools {
		r, _ := e.ExecuteTool(ctx, tool, m)
		out = append(out, r)
	}
	return out, nil
}

// sseEventNames returns the `event:` names in order, as a client would read them.
func sseEventNames(body string) []string {
	var names []string
	for line := range strings.SplitSeq(body, "\n") {
		if strings.HasPrefix(line, "event: ") {
			names = append(names, strings.TrimPrefix(line, "event: "))
		}
	}
	return names
}

func countName(names []string, want string) int {
	n := 0
	for _, name := range names {
		if name == want {
			n++
		}
	}
	return n
}

func runTwoRoundStream(t *testing.T) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/", nil)

	interceptor := NewGenericStreamInterceptor(
		c, noopServerOps{}, &typ.Provider{UUID: "p"}, nil, coretool.NewVirtualToolRegistry(), nil,
		NewAnthropicBetaAdapter(),
		&roundsForwarder{rounds: [][]anthropic.BetaRawMessageStreamEventUnion{
			toolRoundEvents(t), textRoundEvents(t),
		}},
		cannedExecutor{text: "tool output"},
		InterceptorConfig{MaxRounds: 3},
	)
	if err := interceptor.Run(&anthropic.BetaMessageNewParams{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return rec.Body.String()
}

// A multi-round loop must still produce ONE well-formed message. The client
// asked for a single completion; that a tool round happened inside is exactly
// what the loop is supposed to hide.
func TestStreamInterceptor_MultiRoundEmitsOneMessageEnvelope(t *testing.T) {
	body := runTwoRoundStream(t)
	names := sseEventNames(body)

	if got := countName(names, "message_start"); got != 1 {
		t.Errorf("message_start emitted %d times, want 1 — a second envelope makes the stream malformed; events=%v", got, names)
	}
	if got := countName(names, "message_stop"); got != 1 {
		t.Errorf("message_stop emitted %d times, want 1; events=%v", got, names)
	}
	if got := countName(names, "message_delta"); got != 1 {
		t.Errorf("message_delta emitted %d times, want 1; events=%v", got, names)
	}
	if names[0] != "message_start" {
		t.Errorf("stream must open with message_start, got %q; events=%v", names[0], names)
	}
	if names[len(names)-1] != "message_stop" {
		t.Errorf("stream must close with message_stop, got %q; events=%v", names[len(names)-1], names)
	}
}

// The terminal stop_reason must describe how the message actually ended, not
// how an intermediate tool round ended. A client that reads "tool_use" looks
// for a tool_use block it will never find — the loop consumed it.
func TestStreamInterceptor_FinalStopReasonComesFromTheLastRound(t *testing.T) {
	body := runTwoRoundStream(t)

	if strings.Contains(body, `"stop_reason":"tool_use"`) {
		t.Errorf("terminal stop_reason leaked the tool round's value; body=%s", body)
	}
	if !strings.Contains(body, `"stop_reason":"end_turn"`) {
		t.Errorf("terminal stop_reason should be the final round's end_turn; body=%s", body)
	}
}

// The tool call itself must never reach the client.
func TestStreamInterceptor_VirtualToolCallIsNotVisibleToTheClient(t *testing.T) {
	body := runTwoRoundStream(t)
	if strings.Contains(body, roundsToolName) {
		t.Errorf("server-executed tool call leaked to the client; body=%s", body)
	}
	if !strings.Contains(body, "Done.") {
		t.Errorf("final answer missing; body=%s", body)
	}
}

// The real-world variant that first surfaced this: some upstreams (the
// in-process vmodel among them) close a round without a stop_reason. The
// terminal event must still not describe the tool round — the client would
// look for a tool_use block the loop already consumed.
func TestStreamInterceptor_FinalRoundWithoutStopReasonDoesNotReuseTheToolRound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/", nil)

	textNoStopReason := anthropicEvents(t,
		`{"type":"message_start","message":{"id":"m2","type":"message","role":"assistant","content":[],"model":"downstream","stop_reason":null,"usage":{"input_tokens":20,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Done."}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":null,"stop_sequence":null},"usage":{"output_tokens":7}}`,
		`{"type":"message_stop"}`,
	)

	interceptor := NewGenericStreamInterceptor(
		c, noopServerOps{}, &typ.Provider{UUID: "p"}, nil, coretool.NewVirtualToolRegistry(), nil,
		NewAnthropicBetaAdapter(),
		&roundsForwarder{rounds: [][]anthropic.BetaRawMessageStreamEventUnion{
			toolRoundEvents(t), textNoStopReason,
		}},
		cannedExecutor{text: "tool output"},
		InterceptorConfig{MaxRounds: 3},
	)
	if err := interceptor.Run(&anthropic.BetaMessageNewParams{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	body := rec.Body.String()

	if strings.Contains(body, `"stop_reason":"tool_use"`) {
		t.Errorf("terminal stop_reason reused the tool round's value; body=%s", body)
	}
	names := sseEventNames(body)
	if got := countName(names, "message_start"); got != 1 {
		t.Errorf("message_start emitted %d times, want 1; events=%v", got, names)
	}
	if names[len(names)-1] != "message_stop" {
		t.Errorf("stream must close with message_stop; events=%v", names)
	}
}

// The same contract on the v1 union. Both are live: which one a request gets
// depends on the target the rule resolves to, and the interceptor's envelope
// bookkeeping used to recognise only the Beta shape — so v1 clients received a
// second message_start while every Beta-based test stayed green.
func TestStreamInterceptor_V1UnionAlsoEmitsOneMessageEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/", nil)

	v1Events := func(raws ...string) []anthropic.MessageStreamEventUnion {
		out := make([]anthropic.MessageStreamEventUnion, 0, len(raws))
		for _, raw := range raws {
			var e anthropic.MessageStreamEventUnion
			if err := json.Unmarshal([]byte(raw), &e); err != nil {
				t.Fatalf("unmarshal %s: %v", raw, err)
			}
			out = append(out, e)
		}
		return out
	}

	toolRound := v1Events(
		`{"type":"message_start","message":{"id":"m1","type":"message","role":"assistant","content":[],"model":"downstream","stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"`+roundsToolName+`","input":{}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"query\":\"go\"}"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":5}}`,
		`{"type":"message_stop"}`,
	)
	textRound := v1Events(
		`{"type":"message_start","message":{"id":"m2","type":"message","role":"assistant","content":[],"model":"downstream","stop_reason":null,"usage":{"input_tokens":20,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Done."}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":7}}`,
		`{"type":"message_stop"}`,
	)

	interceptor := NewGenericStreamInterceptor(
		c, noopServerOps{}, &typ.Provider{UUID: "p"}, nil, coretool.NewVirtualToolRegistry(), nil,
		NewAnthropicV1Adapter(),
		&v1RoundsForwarder{rounds: [][]anthropic.MessageStreamEventUnion{toolRound, textRound}},
		cannedExecutor{text: "tool output"},
		InterceptorConfig{MaxRounds: 3},
	)
	if err := interceptor.Run(&anthropic.MessageNewParams{}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	body := rec.Body.String()
	names := sseEventNames(body)
	if got := countName(names, "message_start"); got != 1 {
		t.Errorf("message_start emitted %d times, want 1; events=%v", got, names)
	}
	if strings.Contains(body, roundsToolName) {
		t.Errorf("server-executed tool call leaked to the client; body=%s", body)
	}
	if strings.Contains(body, `"stop_reason":"tool_use"`) {
		t.Errorf("terminal stop_reason leaked the tool round's value; body=%s", body)
	}
}

type v1SliceStream struct {
	events []anthropic.MessageStreamEventUnion
	idx    int
}

func (s *v1SliceStream) Next() bool   { s.idx++; return s.idx <= len(s.events) }
func (s *v1SliceStream) Current() any { return s.events[s.idx-1] }
func (s *v1SliceStream) Err() error   { return nil }
func (s *v1SliceStream) Close() error { return nil }

type v1RoundsForwarder struct {
	rounds [][]anthropic.MessageStreamEventUnion
	calls  int
}

func (f *v1RoundsForwarder) ForwardStream(context.Context, any, string, any) (StreamHandle, error) {
	events := f.rounds[min(f.calls, len(f.rounds)-1)]
	f.calls++
	return &v1SliceStream{events: events}, nil
}

func (f *v1RoundsForwarder) ForwardNonStream(context.Context, any, string, any) (any, error) {
	return nil, nil
}
