package smart_guide

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/afk"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	anthropicvm "github.com/tingly-dev/tingly-box/vmodel/anthropic"
	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

// This file integration-tests TinglyBoxAgent.ExecuteWithHandler (and the
// engineSink streaming adapter) end to end against the vmodel virtual Anthropic
// server, which emits real Anthropic streaming SSE. It exercises the path the
// unit tests deliberately skipped.

// scriptedModel is a virtual Anthropic model that drives a two-turn ReAct
// exchange: on the first request (no tool_result in history) it emits a
// tool_use block; once it sees a tool_result it emits final text. This lets a
// single model serve a full agent round trip without looping forever.
type scriptedModel struct {
	anthropicvm.VirtualModel
	id        string
	toolName  string
	toolArgs  map[string]any
	finalText string
}

func newScriptedModel(id, toolName string, toolArgs map[string]any, finalText string) *scriptedModel {
	// Embed a static mock for the identity/metadata methods; we override the
	// Handle* methods below.
	base := anthropicvm.NewMockModel(&anthropicvm.MockModelConfig{ID: id, Name: id})
	return &scriptedModel{
		VirtualModel: base,
		id:           id,
		toolName:     toolName,
		toolArgs:     toolArgs,
		finalText:    finalText,
	}
}

// hasToolResult reports whether the request history already contains a
// tool_result block, i.e. the engine has executed our tool and is asking for
// the follow-up turn.
func (m *scriptedModel) hasToolResult(req *protocol.AnthropicBetaMessagesRequest) bool {
	for _, msg := range req.Messages {
		for _, block := range msg.Content {
			if block.OfToolResult != nil {
				return true
			}
		}
	}
	return false
}

func (m *scriptedModel) HandleAnthropic(req *protocol.AnthropicBetaMessagesRequest) (anthropicvm.VModelResponse, error) {
	if m.hasToolResult(req) {
		return anthropicvm.VModelResponse{
			Content:    []sdk.BetaContentBlockParamUnion{{OfText: &sdk.BetaTextBlockParam{Text: m.finalText}}},
			StopReason: sdk.BetaStopReasonEndTurn,
		}, nil
	}
	inputJSON, _ := json.Marshal(m.toolArgs)
	return anthropicvm.VModelResponse{
		Content: []sdk.BetaContentBlockParamUnion{
			{OfToolUse: &sdk.BetaToolUseBlockParam{
				ID:    "toolu_scripted",
				Name:  m.toolName,
				Input: json.RawMessage(inputJSON),
			}},
		},
		StopReason: sdk.BetaStopReasonToolUse,
	}, nil
}

func (m *scriptedModel) HandleAnthropicStream(req *protocol.AnthropicBetaMessagesRequest, emit func(any)) error {
	resp, err := m.HandleAnthropic(req)
	if err != nil {
		return err
	}
	emit(anthropicvm.StreamStartEvent{MsgID: "msg_scripted", Model: m.id})
	for i, blk := range resp.Content {
		switch {
		case blk.OfText != nil:
			emit(anthropicvm.TextDeltaEvent{Index: i, Text: blk.OfText.Text})
		case blk.OfToolUse != nil:
			in, _ := json.Marshal(blk.OfToolUse.Input)
			emit(anthropicvm.ToolUseEvent{Index: i, ID: blk.OfToolUse.ID, Name: blk.OfToolUse.Name, Input: in})
		}
	}
	emit(anthropicvm.DoneEvent{StopReason: string(resp.StopReason)})
	return nil
}

// newVModelServer mounts the vmodel virtualserver at /v1 and registers the
// given models. It returns the base URL the SDK should target (the SDK appends
// /v1/messages).
func newVModelServer(t *testing.T, models ...anthropicvm.VirtualModel) string {
	t.Helper()
	gin.SetMode(gin.TestMode)

	svc := virtualserver.NewService()
	reg := svc.GetAnthropicRegistry()
	for _, m := range models {
		require.NoError(t, reg.Register(m))
	}

	engine := gin.New()
	svc.SetupRoutes(engine.Group("/v1"))
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)
	return srv.URL
}

// recordingHandler captures the map-shaped messages and completion signal that
// the engineSink/ExecuteWithHandler emit.
type recordingHandler struct {
	mu          sync.Mutex
	texts       []string
	toolCalls   []string
	toolResults []string
	completed   *CompletionResult
	errs        []error
}

func (h *recordingHandler) OnMessage(msg any) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	m, ok := msg.(map[string]interface{})
	if !ok {
		return nil
	}
	switch m["type"] {
	case "assistant":
		if s, ok := m["message"].(string); ok {
			h.texts = append(h.texts, s)
		}
	case "tool_use":
		if s, ok := m["name"].(string); ok {
			h.toolCalls = append(h.toolCalls, s)
		}
	case "tool_result":
		if s, ok := m["name"].(string); ok {
			h.toolResults = append(h.toolResults, s)
		}
	}
	return nil
}

func (h *recordingHandler) OnError(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.errs = append(h.errs, err)
}

func (h *recordingHandler) OnComplete(result *CompletionResult) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.completed = result
}

func (h *recordingHandler) joinedText() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return strings.Join(h.texts, "")
}

// newTestAgent builds a TinglyBoxAgent wired to the vmodel server with a custom
// tool set (bypassing BuildTools so tests can inject a recording tool).
func newTestAgent(t *testing.T, baseURL, model string, cfg *AgentConfig) *TinglyBoxAgent {
	t.Helper()
	cfg.BaseURL = baseURL
	cfg.APIKey = "test-key"
	cfg.Model = model
	if cfg.SmartGuideConfig == nil {
		cfg.SmartGuideConfig = DefaultSmartGuideConfig()
	}
	agent, err := NewTinglyBoxAgent(cfg)
	require.NoError(t, err)
	return agent
}

// rebuildEngineWithTools swaps the agent's engine for one wired to the given
// tools, keeping the same gateway endpoint/model. Used to inject a recording
// tool for round-trip assertions. In-package access to the unexported engine
// field makes this a clean, focused override.
func rebuildEngineWithTools(t *testing.T, agent *TinglyBoxAgent, baseURL, model string, tools ...afk.Tool) {
	t.Helper()
	eng, err := afk.NewEngine(afk.Config{
		BaseURL: baseURL,
		APIKey:  "test-key",
		Model:   model,
		Tools:   tools,
	})
	require.NoError(t, err)
	agent.engine = eng
}

// TestExecuteWithHandler_PlainText drives a no-tool turn and asserts streamed
// text, completion, and that history advanced (user + assistant).
func TestExecuteWithHandler_PlainText(t *testing.T) {
	const model = "scripted-plain"
	// A scripted model with no tool args and that immediately returns text:
	// since history has no tool_result, hasToolResult is false, so it would
	// emit a tool_use. To get plain text on turn one, use the built-in static
	// mock instead.
	static := anthropicvm.NewMockModel(&anthropicvm.MockModelConfig{
		ID: model, Name: model, Content: "Hello from smart guide", StopReason: "end_turn",
	})
	baseURL := newVModelServer(t, static)

	agent := newTestAgent(t, baseURL, model, &AgentConfig{ChatID: "c1"})
	handler := &recordingHandler{}
	toolCtx := &ToolContext{ChatID: "c1", SessionID: "c1"}

	res, err := agent.ExecuteWithHandler(context.Background(), "hi", toolCtx, handler)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.IsSuccess())

	assert.Equal(t, "Hello from smart guide", handler.joinedText())
	assert.Equal(t, "Hello from smart guide", res.Output)
	require.NotNil(t, handler.completed)
	assert.True(t, handler.completed.Success)
	assert.Empty(t, handler.errs)

	// History: user + assistant.
	hist := agent.History()
	require.Len(t, hist, 2)
	assert.Equal(t, sdk.MessageParamRoleUser, hist[0].Role)
	assert.Equal(t, sdk.MessageParamRoleAssistant, hist[1].Role)
	assert.Equal(t, "Hello from smart guide", agent.LastAssistantText())
}

// recordingTool is a minimal afk.Tool that records its invocation and returns a
// fixed string.
type recordingTool struct {
	name   string
	called int
	gotArg string
}

func (r *recordingTool) Name() string        { return r.name }
func (r *recordingTool) Description() string { return "test tool" }
func (r *recordingTool) Schema() (map[string]any, []string) {
	return map[string]any{
		"q": map[string]any{"type": "string", "description": "query"},
	}, nil
}
func (r *recordingTool) Call(_ context.Context, raw json.RawMessage) (string, error) {
	r.called++
	var in struct {
		Q string `json:"q"`
	}
	_ = json.Unmarshal(raw, &in)
	r.gotArg = in.Q
	return "tool-output-42", nil
}

// TestExecuteWithHandler_ToolRoundTrip drives a full ReAct round trip: the model
// requests a tool, the engine executes it, the model returns final text. It
// asserts the tool fired, the sink saw tool_use/tool_result, and history has the
// full 4-message shape.
func TestExecuteWithHandler_ToolRoundTrip(t *testing.T) {
	const model = "scripted-tool"
	scripted := newScriptedModel(model, "lookup", map[string]any{"q": "weather"}, "It is sunny.")
	baseURL := newVModelServer(t, scripted)

	// Build an agent, then replace its engine with one wired to a recording
	// tool so we can assert execution. We reuse NewTinglyBoxAgent for the
	// surrounding wiring, then rebuild the engine with our tool.
	tool := &recordingTool{name: "lookup"}
	agent := newTestAgent(t, baseURL, model, &AgentConfig{ChatID: "c2"})
	rebuildEngineWithTools(t, agent, baseURL, model, tool)

	handler := &recordingHandler{}
	toolCtx := &ToolContext{ChatID: "c2", SessionID: "c2"}

	res, err := agent.ExecuteWithHandler(context.Background(), "what's the weather", toolCtx, handler)
	require.NoError(t, err)
	assert.True(t, res.IsSuccess())

	// Tool executed once with the scripted argument.
	assert.Equal(t, 1, tool.called)
	assert.Equal(t, "weather", tool.gotArg)

	// Sink saw the tool activity and the final text.
	assert.Contains(t, handler.toolCalls, "lookup")
	assert.Contains(t, handler.toolResults, "lookup")
	assert.Equal(t, "It is sunny.", handler.joinedText())
	assert.Equal(t, "It is sunny.", res.Output)
	require.NotNil(t, handler.completed)
	assert.True(t, handler.completed.Success)

	// History: user, assistant(tool_use), user(tool_result), assistant(text).
	hist := agent.History()
	require.Len(t, hist, 4)
	assert.Equal(t, sdk.MessageParamRoleUser, hist[0].Role)
	assert.Equal(t, sdk.MessageParamRoleAssistant, hist[1].Role)
	assert.Equal(t, sdk.MessageParamRoleUser, hist[2].Role)
	assert.Equal(t, sdk.MessageParamRoleAssistant, hist[3].Role)
}

// TestExecuteWithHandler_HistoryContinuity verifies that history seeded via
// NewTinglyBoxAgentWithSession is carried into the next turn (the engine
// receives prior context) and that the result extends it.
func TestExecuteWithHandler_HistoryContinuity(t *testing.T) {
	const model = "scripted-cont"
	static := anthropicvm.NewMockModel(&anthropicvm.MockModelConfig{
		ID: model, Name: model, Content: "second", StopReason: "end_turn",
	})
	baseURL := newVModelServer(t, static)

	seed := []sdk.MessageParam{
		sdk.NewUserMessage(sdk.NewTextBlock("first question")),
		sdk.NewAssistantMessage(sdk.NewTextBlock("first answer")),
	}
	cfg := &AgentConfig{ChatID: "c3", BaseURL: baseURL, APIKey: "k", Model: model}
	agent, err := NewTinglyBoxAgentWithSession(cfg, seed)
	require.NoError(t, err)

	handler := &recordingHandler{}
	_, err = agent.ExecuteWithHandler(context.Background(), "second question", &ToolContext{ChatID: "c3"}, handler)
	require.NoError(t, err)

	// Seed (2) + this turn's user + assistant = 4.
	hist := agent.History()
	require.Len(t, hist, 4)
	assert.Equal(t, "second", agent.LastAssistantText())
}

// turnScript describes one assistant turn the multiTurnModel should produce:
// optional leading text, plus an optional tool call. A turn with a tool call is
// not the final turn; a turn without one ends the exchange.
type turnScript struct {
	thinking string
	text     string
	toolName string
	toolArgs map[string]any
}

// multiTurnModel emits a scripted sequence of assistant turns, advancing one
// turn each time it is asked to generate (it counts the tool_result blocks in
// the request history to know which turn it is on). This reproduces the real
// Claude shape where intermediate turns carry BOTH text and a tool_use.
type multiTurnModel struct {
	anthropicvm.VirtualModel
	id    string
	turns []turnScript
}

func newMultiTurnModel(id string, turns ...turnScript) *multiTurnModel {
	base := anthropicvm.NewMockModel(&anthropicvm.MockModelConfig{ID: id, Name: id})
	return &multiTurnModel{VirtualModel: base, id: id, turns: turns}
}

func (m *multiTurnModel) turnIndex(req *protocol.AnthropicBetaMessagesRequest) int {
	n := 0
	for _, msg := range req.Messages {
		for _, block := range msg.Content {
			if block.OfToolResult != nil {
				n++
			}
		}
	}
	return n
}

func (m *multiTurnModel) HandleAnthropic(req *protocol.AnthropicBetaMessagesRequest) (anthropicvm.VModelResponse, error) {
	idx := m.turnIndex(req)
	if idx >= len(m.turns) {
		idx = len(m.turns) - 1
	}
	ts := m.turns[idx]
	var content []sdk.BetaContentBlockParamUnion
	if ts.thinking != "" {
		content = append(content, sdk.BetaContentBlockParamUnion{OfThinking: &sdk.BetaThinkingBlockParam{Thinking: ts.thinking}})
	}
	if ts.text != "" {
		content = append(content, sdk.BetaContentBlockParamUnion{OfText: &sdk.BetaTextBlockParam{Text: ts.text}})
	}
	stop := sdk.BetaStopReasonEndTurn
	if ts.toolName != "" {
		in, _ := json.Marshal(ts.toolArgs)
		content = append(content, sdk.BetaContentBlockParamUnion{OfToolUse: &sdk.BetaToolUseBlockParam{
			ID: "toolu_mt", Name: ts.toolName, Input: json.RawMessage(in),
		}})
		stop = sdk.BetaStopReasonToolUse
	}
	return anthropicvm.VModelResponse{Content: content, StopReason: stop}, nil
}

func (m *multiTurnModel) HandleAnthropicStream(req *protocol.AnthropicBetaMessagesRequest, emit func(any)) error {
	resp, err := m.HandleAnthropic(req)
	if err != nil {
		return err
	}
	emit(anthropicvm.StreamStartEvent{MsgID: "msg_mt", Model: m.id})
	for i, blk := range resp.Content {
		switch {
		case blk.OfThinking != nil:
			emit(anthropicvm.ThinkingDeltaEvent{Index: i, Thinking: blk.OfThinking.Thinking})
		case blk.OfText != nil:
			emit(anthropicvm.TextDeltaEvent{Index: i, Text: blk.OfText.Text})
		case blk.OfToolUse != nil:
			in, _ := json.Marshal(blk.OfToolUse.Input)
			emit(anthropicvm.ToolUseEvent{Index: i, ID: blk.OfToolUse.ID, Name: blk.OfToolUse.Name, Input: in})
		}
	}
	emit(anthropicvm.DoneEvent{StopReason: string(resp.StopReason)})
	return nil
}

// TestExecuteWithHandler_MultiTurnTextNotLost is the regression for "messages
// get lost": a real exchange where the intermediate turn carries BOTH text and
// a tool call, then a final text turn. Every assistant text segment must reach
// the handler — none may be dropped just because the turn also called a tool.
func TestExecuteWithHandler_MultiTurnTextNotLost(t *testing.T) {
	const model = "multi-turn"
	mt := newMultiTurnModel(model,
		turnScript{text: "Let me check that for you.", toolName: "lookup", toolArgs: map[string]any{"q": "x"}},
		turnScript{text: "All done — here is the answer."},
	)
	baseURL := newVModelServer(t, mt)

	tool := &recordingTool{name: "lookup"}
	agent := newTestAgent(t, baseURL, model, &AgentConfig{ChatID: "mt1"})
	rebuildEngineWithTools(t, agent, baseURL, model, tool)

	handler := &recordingHandler{}
	_, err := agent.ExecuteWithHandler(context.Background(), "do a thing", &ToolContext{ChatID: "mt1"}, handler)
	require.NoError(t, err)

	assert.Equal(t, 1, tool.called, "tool should have run once")
	// BOTH text segments must have been delivered — the intermediate turn's
	// text must not be swallowed by the tool-call handling.
	joined := handler.joinedText()
	assert.Contains(t, joined, "Let me check that for you.", "intermediate-turn text was lost")
	assert.Contains(t, joined, "All done — here is the answer.", "final-turn text was lost")
}

// TestExecuteWithHandler_ThinkingSurfacesWhenNoText is the end-to-end test for
// the thinking fallback: when a turn produces only a thinking block (no text)
// the user must still see something. Here the intermediate turn is thinking +
// tool, and the final turn is thinking only — neither has a text block, so
// without the fallback the user would receive nothing at all.
func TestExecuteWithHandler_ThinkingSurfacesWhenNoText(t *testing.T) {
	const model = "thinking-only"
	mt := newMultiTurnModel(model,
		turnScript{thinking: "I should look this up first.", toolName: "lookup", toolArgs: map[string]any{"q": "x"}},
		turnScript{thinking: "Based on the result, the answer is 42."},
	)
	baseURL := newVModelServer(t, mt)

	tool := &recordingTool{name: "lookup"}
	agent := newTestAgent(t, baseURL, model, &AgentConfig{ChatID: "think1"})
	rebuildEngineWithTools(t, agent, baseURL, model, tool)

	handler := &recordingHandler{}
	res, err := agent.ExecuteWithHandler(context.Background(), "answer me", &ToolContext{ChatID: "think1"}, handler)
	require.NoError(t, err)
	assert.True(t, res.IsSuccess())
	assert.Equal(t, 1, tool.called, "tool should have run once")

	// Both turns' thinking surfaced to the user even though neither had text.
	joined := handler.joinedText()
	assert.Contains(t, joined, "I should look this up first.", "intermediate thinking was lost")
	assert.Contains(t, joined, "Based on the result, the answer is 42.", "final thinking was lost")
	// The final response is the thinking fallback, not empty.
	assert.Contains(t, res.Output, "the answer is 42")
}
