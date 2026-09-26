package toolround

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolstream "github.com/tingly-dev/tingly-box/internal/protocol/stream"
)

// ─── scripted provider ─────────────────────────────────────────────────────

// scriptedEndpoint answers round i with rounds[i] (a Beta message JSON),
// either whole or as the event stream a provider would send, and records
// every request it receives.
type scriptedEndpoint struct {
	rounds   []string
	requests []anthropic.BetaMessageNewParams
	failAt   int // 1-based round that fails; 0 never
}

func (e *scriptedEndpoint) Protocol() protocol.APIType { return protocol.TypeAnthropicBeta }

func (e *scriptedEndpoint) round(call stage.Call) (string, error) {
	request := *call.Request.(*anthropic.BetaMessageNewParams)
	e.requests = append(e.requests, request)
	n := len(e.requests)
	if n == e.failAt {
		return "", fmt.Errorf("provider failed in round %d", n)
	}
	if n > len(e.rounds) {
		return e.rounds[len(e.rounds)-1], nil
	}
	return e.rounds[n-1], nil
}

func (e *scriptedEndpoint) Complete(_ context.Context, call stage.Call) (*stage.Response, error) {
	raw, err := e.round(call)
	if err != nil {
		return nil, err
	}
	var message anthropic.BetaMessage
	if err := json.Unmarshal([]byte(raw), &message); err != nil {
		return nil, err
	}
	return &stage.Response{Value: &message, Usage: &protocol.TokenUsage{InputTokens: 10, OutputTokens: 5}, Model: string(message.Model)}, nil
}

func (e *scriptedEndpoint) Stream(_ context.Context, call stage.Call) (stage.EventStream, error) {
	raw, err := e.round(call)
	if err != nil {
		return nil, err
	}
	return &scriptedStream{events: messageEvents(raw)}, nil
}

type scriptedStream struct {
	events []stage.Event
	next   int
}

func (s *scriptedStream) Next(context.Context) (stage.Event, error) {
	if s.next >= len(s.events) {
		return stage.Event{}, io.EOF
	}
	s.next++
	return s.events[s.next-1], nil
}

func (s *scriptedStream) Close() error { return nil }

func (s *scriptedStream) Result() stage.StreamResult {
	return stage.StreamResult{Usage: &protocol.TokenUsage{InputTokens: 10, OutputTokens: 5}, Model: "provider-model"}
}

// messageEvents renders a Beta message JSON as a provider stream. Tool input
// arrives in two partial_json chunks, as real providers split it.
func messageEvents(raw string) []stage.Event {
	var lines []string
	start, _ := setJSON(raw, "content", "[]")
	start, _ = setJSON(start, "stop_reason", "null")
	lines = append(lines, `{"type":"message_start","message":`+start+`}`)
	for i, block := range gjson.Get(raw, "content").Array() {
		switch block.Get("type").String() {
		case "text":
			lines = append(lines,
				fmt.Sprintf(`{"type":"content_block_start","index":%d,"content_block":{"type":"text","text":""}}`, i),
				fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"text_delta","text":%s}}`, i, block.Get("text").Raw))
		case "thinking":
			lines = append(lines,
				fmt.Sprintf(`{"type":"content_block_start","index":%d,"content_block":{"type":"thinking","thinking":"","signature":""}}`, i),
				fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"thinking_delta","thinking":%s}}`, i, block.Get("thinking").Raw),
				fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"signature_delta","signature":%s}}`, i, block.Get("signature").Raw))
		case "tool_use":
			input := block.Get("input").Raw
			half := len(input) / 2
			first, _ := json.Marshal(input[:half])
			second, _ := json.Marshal(input[half:])
			lines = append(lines,
				fmt.Sprintf(`{"type":"content_block_start","index":%d,"content_block":{"type":"tool_use","id":%s,"name":%s,"input":{}}}`, i, block.Get("id").Raw, block.Get("name").Raw),
				fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"input_json_delta","partial_json":%s}}`, i, first),
				fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"input_json_delta","partial_json":%s}}`, i, second))
		}
		lines = append(lines, fmt.Sprintf(`{"type":"content_block_stop","index":%d}`, i))
	}
	lines = append(lines,
		fmt.Sprintf(`{"type":"message_delta","delta":{"stop_reason":%s,"stop_sequence":null},"usage":{"output_tokens":5}}`, gjson.Get(raw, "stop_reason").Raw),
		`{"type":"message_stop"}`)
	events := make([]stage.Event, len(lines))
	for i, line := range lines {
		var event anthropic.BetaRawMessageStreamEventUnion
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			panic(err)
		}
		events[i] = stage.Event{Value: event}
	}
	return events
}

func setJSON(raw, path, value string) (string, error) {
	var generic map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &generic); err != nil {
		return "", err
	}
	generic[path] = json.RawMessage(value)
	out, err := json.Marshal(generic)
	return string(out), err
}

func message(stopReason string, blocks ...string) string {
	return fmt.Sprintf(`{"id":"msg_1","type":"message","role":"assistant","model":"provider-model","content":[%s],"stop_reason":%q,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":5}}`,
		strings.Join(blocks, ","), stopReason)
}

func text(s string) string { return fmt.Sprintf(`{"type":"text","text":%q}`, s) }

func toolUse(id, name, input string) string {
	return fmt.Sprintf(`{"type":"tool_use","id":%q,"name":%q,"input":%s}`, id, name, input)
}

func thinking(s, signature string) string {
	return fmt.Sprintf(`{"type":"thinking","thinking":%q,"signature":%q}`, s, signature)
}

// ─── fakes ─────────────────────────────────────────────────────────────────

// fakeOwner owns every tool named srv_*, answering with its input.
type fakeOwner struct {
	executed  []ToolCall
	suspended []anthropic.BetaMessageParam
	result    string
}

func (o *fakeOwner) Owns(name string) bool { return strings.HasPrefix(name, "srv_") }

func (o *fakeOwner) Execute(ctx context.Context, call ToolCall, _ *anthropic.BetaMessageNewParams) (context.Context, anthropic.BetaToolResultBlockParam) {
	o.executed = append(o.executed, call)
	result := o.result
	if result == "" {
		result = "result:" + string(call.Input)
	}
	return ctx, anthropic.BetaToolResultBlockParam{
		ToolUseID: call.ID,
		Content:   []anthropic.BetaToolResultBlockParamContentUnion{{OfText: &anthropic.BetaTextBlockParam{Text: result}}},
	}
}

func (o *fakeOwner) Resume(_ context.Context, request *anthropic.BetaMessageNewParams) *anthropic.BetaMessageNewParams {
	return request
}

func (o *fakeOwner) Suspend(_ context.Context, turn anthropic.BetaMessageParam, results []anthropic.BetaToolResultBlockParam) {
	o.suspended = append(o.suspended, turn, resultMessage(results))
}

// fakeGate blocks tools named "danger", aliases SECRET as ALIAS in tool
// results, and restores ALIAS to REAL in tool inputs.
type fakeGate struct {
	decided   []string
	requests  int
	responses int
}

func (g *fakeGate) Request(context.Context, *anthropic.BetaMessageNewParams) error {
	g.requests++
	return nil
}

func (g *fakeGate) ToolUse(_ context.Context, _ *anthropic.BetaMessageNewParams, call ToolCall) Verdict {
	g.decided = append(g.decided, call.Name)
	if call.Name == "danger" {
		return Verdict{Block: true, Message: "blocked: danger"}
	}
	return Verdict{}
}

func (g *fakeGate) ToolResult(_ context.Context, _ *anthropic.BetaMessageNewParams, _ ToolCall, result *anthropic.BetaToolResultBlockParam) {
	for _, content := range result.Content {
		if content.OfText != nil {
			content.OfText.Text = strings.ReplaceAll(content.OfText.Text, "SECRET", "ALIAS")
		}
	}
}

func (g *fakeGate) Restore(input json.RawMessage) json.RawMessage {
	return json.RawMessage(strings.ReplaceAll(string(input), "ALIAS", "REAL"))
}

func (g *fakeGate) Response(context.Context, *anthropic.BetaMessageNewParams, *anthropic.BetaMessage) {
	g.responses++
}

// ─── runner ────────────────────────────────────────────────────────────────

type outcome struct {
	message    *anthropic.BetaMessage // what the client receives
	committed  bool
	usage      *protocol.TokenUsage
	heartbeats int // streamed only: one per round that runs server tools
}

func clientRequest() *anthropic.BetaMessageNewParams {
	return &anthropic.BetaMessageNewParams{
		Model:     "client-model",
		MaxTokens: 1024,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("go"))},
	}
}

func run(t *testing.T, config Config, provider *scriptedEndpoint, streaming bool) (outcome, error) {
	t.Helper()
	endpoint, err := stage.Compose(provider, New(config))
	require.NoError(t, err)
	call := stage.Call{Request: clientRequest()}
	if !streaming {
		response, err := endpoint.Complete(context.Background(), call)
		if err != nil {
			return outcome{}, err
		}
		return outcome{message: response.Value.(*anthropic.BetaMessage), committed: response.SideEffectsCommitted, usage: response.Usage}, nil
	}

	events, err := endpoint.Stream(context.Background(), call)
	if err != nil {
		return outcome{}, err
	}
	defer events.Close()
	var assembled anthropic.BetaMessage
	starts, nextIndex, heartbeats := 0, int64(0), 0
	for {
		event, err := events.Next(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			result := events.Result()
			return outcome{committed: result.SideEffectsCommitted}, err
		}
		if _, ok := event.Value.(stage.Heartbeat); ok {
			heartbeats++
			continue
		}
		beta := event.Value.(anthropic.BetaRawMessageStreamEventUnion)
		switch beta.Type {
		case "message_start":
			starts++
		case "content_block_start":
			require.Equal(t, nextIndex, beta.Index, "client block indices must be contiguous")
			nextIndex++
		}
		require.NoError(t, assembled.Accumulate(beta))
	}
	require.Equal(t, 1, starts, "one client message per request")
	result := events.Result()
	return outcome{message: &assembled, committed: result.SideEffectsCommitted, usage: result.Usage, heartbeats: heartbeats}, nil
}

func contentTypes(message *anthropic.BetaMessage) []string {
	var types []string
	for _, block := range message.Content {
		types = append(types, block.Type)
	}
	return types
}

func both(t *testing.T, f func(t *testing.T, streaming bool)) {
	t.Run("complete", func(t *testing.T) { f(t, false) })
	t.Run("stream", func(t *testing.T) { f(t, true) })
}

// ─── tests ─────────────────────────────────────────────────────────────────

func TestPassThroughWithoutOwnerOrGate(t *testing.T) {
	provider := &scriptedEndpoint{}
	require.Same(t, stage.Endpoint(provider), New(Config{}).Wrap(provider))
}

func TestNoToolsIsUnchanged(t *testing.T) {
	both(t, func(t *testing.T, streaming bool) {
		provider := &scriptedEndpoint{rounds: []string{message("end_turn", text("Paris"))}}
		out, err := run(t, Config{Owner: &fakeOwner{}, Gate: &fakeGate{}}, provider, streaming)
		require.NoError(t, err)
		require.Len(t, provider.requests, 1)
		require.Equal(t, []string{"text"}, contentTypes(out.message))
		require.Equal(t, "Paris", out.message.Content[0].Text)
		require.Equal(t, anthropic.BetaStopReasonEndTurn, out.message.StopReason)
		require.False(t, out.committed)
	})
}

func TestOwnedToolLoop(t *testing.T) {
	both(t, func(t *testing.T, streaming bool) {
		owner := &fakeOwner{}
		provider := &scriptedEndpoint{rounds: []string{
			message("tool_use", thinking("plan", "sig-1"), text("checking"), toolUse("toolu_1", "srv_echo", `{"q":"hi"}`)),
			message("end_turn", text("final")),
		}}
		out, err := run(t, Config{Owner: owner}, provider, streaming)
		require.NoError(t, err)

		require.Len(t, owner.executed, 1)
		require.JSONEq(t, `{"q":"hi"}`, string(owner.executed[0].Input))
		require.Len(t, provider.requests, 2)
		follow := provider.requests[1].Messages
		require.Len(t, follow, 3)
		turn, err := json.Marshal(follow[1])
		require.NoError(t, err)
		require.Equal(t, "sig-1", gjson.GetBytes(turn, "content.0.signature").String(), "thinking signature must survive into the next round")
		require.Equal(t, "toolu_1", gjson.GetBytes(turn, "content.2.id").String())
		results, err := json.Marshal(follow[2])
		require.NoError(t, err)
		require.Equal(t, "toolu_1", gjson.GetBytes(results, "content.0.tool_use_id").String())

		require.True(t, out.committed)
		require.Equal(t, 20, out.usage.InputTokens, "usage covers every round")
		if streaming {
			require.Equal(t, 1, out.heartbeats, "the client hears from the stream while the tool runs")
		}
		require.Equal(t, anthropic.BetaStopReasonEndTurn, out.message.StopReason)
		if streaming {
			// Streamed rounds reach the client as one message, owned call hidden.
			require.Equal(t, []string{"thinking", "text", "text"}, contentTypes(out.message))
			require.Equal(t, "final", out.message.Content[2].Text)
		} else {
			require.Equal(t, []string{"text"}, contentTypes(out.message))
			require.Equal(t, "final", out.message.Content[0].Text)
		}
	})
}

func TestClientToolPasses(t *testing.T) {
	both(t, func(t *testing.T, streaming bool) {
		provider := &scriptedEndpoint{rounds: []string{
			message("tool_use", text("let me look"), toolUse("toolu_c", "get_weather", `{"location":"Paris"}`)),
		}}
		out, err := run(t, Config{Owner: &fakeOwner{}}, provider, streaming)
		require.NoError(t, err)
		require.Equal(t, []string{"text", "tool_use"}, contentTypes(out.message))
		require.Equal(t, "toolu_c", out.message.Content[1].ID)
		require.JSONEq(t, `{"location":"Paris"}`, string(toolInput(out.message.Content[1])))
		require.Equal(t, anthropic.BetaStopReasonToolUse, out.message.StopReason)
		require.False(t, out.committed)
	})
}

func TestMixedRoundSuspendsOwnedResults(t *testing.T) {
	both(t, func(t *testing.T, streaming bool) {
		owner := &fakeOwner{}
		provider := &scriptedEndpoint{rounds: []string{
			message("tool_use", toolUse("toolu_s", "srv_echo", `{"q":"x"}`), toolUse("toolu_c", "get_weather", `{"location":"Paris"}`)),
		}}
		out, err := run(t, Config{Owner: owner}, provider, streaming)
		require.NoError(t, err)
		require.Len(t, provider.requests, 1)
		require.Len(t, owner.executed, 1)
		require.Len(t, owner.suspended, 2, "assistant turn and owned results are stored")
		stored, err := json.Marshal(owner.suspended[1])
		require.NoError(t, err)
		require.Equal(t, "toolu_s", gjson.GetBytes(stored, "content.0.tool_use_id").String())

		require.Equal(t, []string{"tool_use"}, contentTypes(out.message))
		require.Equal(t, "toolu_c", out.message.Content[0].ID)
		require.Equal(t, anthropic.BetaStopReasonToolUse, out.message.StopReason)
		require.True(t, out.committed)
	})
}

func TestGateBlockEndsTheTurn(t *testing.T) {
	both(t, func(t *testing.T, streaming bool) {
		owner, gate := &fakeOwner{}, &fakeGate{}
		provider := &scriptedEndpoint{rounds: []string{
			message("tool_use", text("sure"), toolUse("toolu_s", "srv_echo", `{}`), toolUse("toolu_d", "danger", `{"cmd":"rm -rf /"}`), toolUse("toolu_c", "get_weather", `{}`)),
		}}
		out, err := run(t, Config{Owner: owner, Gate: gate}, provider, streaming)
		require.NoError(t, err)
		require.Equal(t, []string{"srv_echo", "danger", "get_weather"}, gate.decided, "every call is gated, owned ones included")
		require.Empty(t, owner.executed, "nothing in a blocked round executes")
		require.Len(t, provider.requests, 1)
		require.Equal(t, []string{"text", "text"}, contentTypes(out.message))
		require.Equal(t, "blocked: danger", out.message.Content[1].Text)
		require.Equal(t, anthropic.BetaStopReasonEndTurn, out.message.StopReason)
		require.False(t, out.committed)
	})
}

func TestRoundLimitEndsWithoutExposingOwnedCalls(t *testing.T) {
	both(t, func(t *testing.T, streaming bool) {
		owner := &fakeOwner{}
		provider := &scriptedEndpoint{rounds: []string{
			message("tool_use", text("again"), toolUse("toolu_s", "srv_echo", `{}`)),
		}}
		out, err := run(t, Config{Owner: owner}, provider, streaming)
		require.NoError(t, err)
		require.Len(t, owner.executed, DefaultMaxRounds)
		require.Len(t, provider.requests, DefaultMaxRounds+1)
		for _, block := range out.message.Content {
			require.NotEqual(t, "tool_use", block.Type)
		}
		require.Equal(t, anthropic.BetaStopReasonEndTurn, out.message.StopReason)
		require.True(t, out.committed)
	})
}

func TestGateRestoresAndScreens(t *testing.T) {
	both(t, func(t *testing.T, streaming bool) {
		owner := &fakeOwner{result: "token SECRET"}
		provider := &scriptedEndpoint{rounds: []string{
			message("tool_use", toolUse("toolu_s", "srv_echo", `{"key":"ALIAS"}`)),
			message("tool_use", toolUse("toolu_c", "get_weather", `{"key":"ALIAS"}`)),
		}}
		gate := &fakeGate{}
		out, err := run(t, Config{Owner: owner, Gate: gate}, provider, streaming)
		require.NoError(t, err)
		require.Equal(t, 1, gate.requests)

		require.JSONEq(t, `{"key":"REAL"}`, string(owner.executed[0].Input), "owned tools run on real values")
		fedBack, err := json.Marshal(provider.requests[1].Messages[2])
		require.NoError(t, err)
		require.Contains(t, string(fedBack), "token ALIAS", "tool results are screened before the model sees them")
		require.NotContains(t, string(fedBack), "SECRET")

		require.JSONEq(t, `{"key":"REAL"}`, string(toolInput(out.message.Content[len(out.message.Content)-1])), "client tools receive real values")
		if !streaming {
			require.Equal(t, 1, gate.responses)
		}
	})
}

func TestErrorsAfterExecutionAreCommitted(t *testing.T) {
	both(t, func(t *testing.T, streaming bool) {
		loop := []string{
			message("tool_use", toolUse("toolu_s", "srv_echo", `{}`)),
			message("end_turn", text("final")),
		}
		_, err := run(t, Config{Owner: &fakeOwner{}}, &scriptedEndpoint{rounds: loop, failAt: 1}, streaming)
		require.Error(t, err)
		require.False(t, stage.HasCommittedSideEffects(err), "nothing ran: failover may retry")

		_, err = run(t, Config{Owner: &fakeOwner{}}, &scriptedEndpoint{rounds: loop, failAt: 2}, streaming)
		require.Error(t, err)
		require.True(t, stage.HasCommittedSideEffects(err), "a server tool ran: failover must not retry")
	})
}

// Bridges below the stage may emit carrier events; the stage reads their wire
// payload.
func TestAcceptsBridgeCarrierEvents(t *testing.T) {
	event, err := betaEvent(protocolstream.AnthropicEvent{Type: "message_stop", Data: map[string]any{"type": "message_stop"}})
	require.NoError(t, err)
	require.Equal(t, "message_stop", event.Type)

	_, err = betaEvent(42)
	require.Error(t, err)
}

func TestRejectsForeignRequest(t *testing.T) {
	endpoint := New(Config{Owner: &fakeOwner{}}).Wrap(&scriptedEndpoint{})
	_, err := endpoint.Complete(context.Background(), stage.Call{Request: &anthropic.MessageNewParams{}})
	require.ErrorContains(t, err, "want anthropic.BetaMessageNewParams")
}
