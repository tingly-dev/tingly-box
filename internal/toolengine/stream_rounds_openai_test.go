package toolengine

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"

	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// OpenAI Chat counterpart of the two-round fixtures: the stream handle yields
// SDK chunks by value, exactly as OpenAIChatStreamHandle does.

func openAIChunks(t *testing.T, raws ...string) []openai.ChatCompletionChunk {
	t.Helper()
	out := make([]openai.ChatCompletionChunk, 0, len(raws))
	for _, raw := range raws {
		var c openai.ChatCompletionChunk
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			t.Fatalf("unmarshal chunk %s: %v", raw, err)
		}
		out = append(out, c)
	}
	return out
}

type chunkStream struct {
	chunks []openai.ChatCompletionChunk
	idx    int
}

func (s *chunkStream) Next() bool   { s.idx++; return s.idx <= len(s.chunks) }
func (s *chunkStream) Current() any { return s.chunks[s.idx-1] }
func (s *chunkStream) Err() error   { return nil }
func (s *chunkStream) Close() error { return nil }

type chunkRoundsForwarder struct {
	rounds [][]openai.ChatCompletionChunk
	calls  int
}

func (f *chunkRoundsForwarder) ForwardStream(context.Context, any, string, any) (StreamHandle, error) {
	chunks := f.rounds[min(f.calls, len(f.rounds)-1)]
	f.calls++
	return &chunkStream{chunks: chunks}, nil
}

func (f *chunkRoundsForwarder) ForwardNonStream(context.Context, any, string, any) (any, error) {
	return nil, nil
}

// The final round's text must reach the client after a server tool round.
func TestStreamInterceptor_OpenAIChatFinalTextReachesClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/", nil)

	forwarder := &chunkRoundsForwarder{rounds: [][]openai.ChatCompletionChunk{
		openAIChunks(t,
			`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"`+roundsToolName+`","arguments":"{\"query\":\"go\"}"}}]},"finish_reason":null}]}`,
			`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		),
		openAIChunks(t,
			`{"id":"c2","object":"chat.completion.chunk","created":2,"model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":"Done."},"finish_reason":null}]}`,
			`{"id":"c2","object":"chat.completion.chunk","created":2,"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		),
	}}
	// Production registers every server tool; chunk suppression keys off it.
	registry := coretool.NewVirtualToolRegistry()
	registry.Register(coretool.VirtualTool{Name: "advisor"})
	interceptor := NewGenericStreamInterceptor(
		c, noopServerOps{}, &typ.Provider{UUID: "p"}, nil, registry, nil,
		NewOpenAIChatAdapter(), forwarder, cannedExecutor{text: "tool output"},
		InterceptorConfig{MaxRounds: 3},
	)
	if err := interceptor.Run(&openai.ChatCompletionNewParams{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	body := rec.Body.String()

	if forwarder.calls != 2 {
		t.Fatalf("upstream rounds = %d, want 2", forwarder.calls)
	}
	if !strings.Contains(body, `"content":"Done."`) {
		t.Errorf("final answer missing from client stream:\n%s", body)
	}
	for line := range strings.SplitSeq(body, "\n") {
		if strings.TrimSpace(line) == "data:" {
			t.Errorf("client stream carries an empty data frame:\n%s", body)
			break
		}
	}
	if strings.Contains(body, roundsToolName) {
		t.Errorf("server tool call leaked to client:\n%s", body)
	}
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

// A server-executed tool name. advisor is the built-in one every classifier
// already recognises, so the harness needs no registry setup.
const roundsToolName = "tingly_box_mcp__advisor__advisor"
