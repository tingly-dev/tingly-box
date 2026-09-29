package toolengine

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"

	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

var errUpstream = errors.New("upstream 503")

// failingForwarder answers the first rounds from rounds / completions and
// fails every call after them.
type failingForwarder struct {
	rounds      [][]openai.ChatCompletionChunk
	completions []*openai.ChatCompletion
	calls       int
}

func (f *failingForwarder) ForwardStream(context.Context, any, string, any) (StreamHandle, error) {
	defer func() { f.calls++ }()
	if f.calls < len(f.rounds) {
		return &chunkStream{chunks: f.rounds[f.calls]}, nil
	}
	return nil, errUpstream
}

func (f *failingForwarder) ForwardNonStream(context.Context, any, string, any) (any, error) {
	defer func() { f.calls++ }()
	if f.calls < len(f.completions) {
		return f.completions[f.calls], nil
	}
	return nil, errUpstream
}

func serverToolRegistry() *coretool.VirtualToolRegistry {
	registry := coretool.NewVirtualToolRegistry()
	registry.Register(coretool.VirtualTool{Name: "advisor"})
	return registry
}

func serverToolCompletion(t *testing.T) *openai.ChatCompletion {
	t.Helper()
	var completion openai.ChatCompletion
	raw := `{"id":"c1","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"` + roundsToolName + `","arguments":"{\"query\":\"go\"}"}}]}}]}`
	if err := json.Unmarshal([]byte(raw), &completion); err != nil {
		t.Fatalf("unmarshal completion: %v", err)
	}
	return &completion
}

// An upstream failure is committed only once a server tool has run: before
// that, failover may retry the request; after it, a retry would run the
// tool again.
func TestGenericLoopCommitsErrorsAfterServerTool(t *testing.T) {
	cases := []struct {
		name          string
		toolRound     bool
		wantCommitted bool
	}{
		{"failure before any tool", false, false},
		{"failure after a server tool", true, true},
	}
	for _, tc := range cases {
		t.Run("stream/"+tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/", nil)
			forwarder := &failingForwarder{}
			if tc.toolRound {
				forwarder.rounds = [][]openai.ChatCompletionChunk{openAIChunks(t,
					`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"`+roundsToolName+`","arguments":"{\"query\":\"go\"}"}}]},"finish_reason":null}]}`,
					`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
				)}
			}
			interceptor := NewGenericStreamInterceptor(
				c, noopServerOps{}, &typ.Provider{UUID: "p"}, nil, serverToolRegistry(), nil,
				NewOpenAIChatAdapter(), forwarder, cannedExecutor{text: "tool output"},
				InterceptorConfig{MaxRounds: 3},
			)
			err := interceptor.Run(&openai.ChatCompletionNewParams{})
			if !errors.Is(err, errUpstream) {
				t.Fatalf("Run error = %v, want the upstream failure", err)
			}
			if got := stage.HasCommittedSideEffects(err); got != tc.wantCommitted {
				t.Errorf("committed = %v, want %v", got, tc.wantCommitted)
			}
		})
		t.Run("nonstream/"+tc.name, func(t *testing.T) {
			forwarder := &failingForwarder{}
			if tc.toolRound {
				forwarder.completions = []*openai.ChatCompletion{serverToolCompletion(t)}
			}
			processor := NewGenericLoopProcessor(
				context.Background(), noopServerOps{}, &typ.Provider{UUID: "p"}, nil, serverToolRegistry(), nil,
				NewOpenAIChatAdapter(), forwarder, cannedExecutor{text: "tool output"},
				InterceptorConfig{MaxRounds: 3},
			)
			_, err := processor.Run(&openai.ChatCompletionNewParams{})
			if !errors.Is(err, errUpstream) {
				t.Fatalf("Run error = %v, want the upstream failure", err)
			}
			if got := stage.HasCommittedSideEffects(err); got != tc.wantCommitted {
				t.Errorf("committed = %v, want %v", got, tc.wantCommitted)
			}
		})
	}
}
