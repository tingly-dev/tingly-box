package protocoltest

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

	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/anthropicbridge"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/toolround"
	"github.com/tingly-dev/tingly-box/internal/protocolserver/servertool"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/toolengine"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// The Tool Round Stage runs in memory over the same server-tool fixtures as
// the HTTP MCP tests (mcp_loop_test.go), with the real MCP runtime, servertool
// pipeline and toolengine owner, on top of each provider protocol through the
// P2 bridges. It checks the behavior the HTTP tests pin, including the cases
// registered there as gaps (M4 round limit on Chat, M5 mixed continuation on
// Chat), which the stage handles once for every target.

// recordingEndpoint records every request a terminal receives, in wire JSON.
type recordingEndpoint struct {
	stage.Endpoint
	requests []string
}

func (e *recordingEndpoint) Complete(ctx context.Context, call stage.Call) (*stage.Response, error) {
	e.requests = append(e.requests, string(mustMarshal(call.Request)))
	return e.Endpoint.Complete(ctx, call)
}

func (e *recordingEndpoint) Stream(ctx context.Context, call stage.Call) (stage.EventStream, error) {
	e.requests = append(e.requests, string(mustMarshal(call.Request)))
	return e.Endpoint.Stream(ctx, call)
}

// runtimeExecutor is the ToolExecutorServer the gateway builds per request:
// the servertool pipeline over the MCP runtime.
type runtimeExecutor struct {
	runtime  *mcpruntime.Runtime
	pipeline *servertool.Pipeline
}

func (r runtimeExecutor) CallMCPToolWithHooks(ctx context.Context, name, arguments string, messages []map[string]any) (context.Context, coretool.ToolResult, error) {
	return r.pipeline.NewExecutor(r.runtime, r.runtime).Execute(ctx, servertool.ToolCall{NormalizedName: name, Arguments: arguments, Messages: messages})
}

func (r runtimeExecutor) CallMCPTool(ctx context.Context, name, arguments string, messages []map[string]any) (string, error) {
	_, result, err := r.CallMCPToolWithHooks(ctx, name, arguments, messages)
	var text strings.Builder
	for _, content := range result.Contents {
		text.WriteString(content.Text)
	}
	return text.String(), err
}

type toolRoundHarness struct {
	echo     *EchoServertoolProvider
	terminal *recordingEndpoint
	endpoint stage.Endpoint
}

func newToolRoundHarness(t *testing.T, echo *EchoServertoolProvider, target protocol.APIType, s Scenario) *toolRoundHarness {
	t.Helper()
	runtime := mcpruntime.NewRuntime(func() *typ.MCPRuntimeConfig { return &typ.MCPRuntimeConfig{} })
	t.Cleanup(runtime.Close)
	pipeline := servertool.NewPipeline()
	pipeline.Register(echo)
	pipeline.RegisterInto(runtime.VirtualRegistry())
	owner := toolengine.NewAnthropicBetaOwner(runtime.VirtualRegistry(), toolengine.NewServerToolExecutor(runtimeExecutor{runtime, pipeline}), "provider-"+string(target))

	h := &toolRoundHarness{echo: echo}
	h.terminal = &recordingEndpoint{Endpoint: &fixtureEndpoint{protocol: target, mock: s.MockResponses[targetFormat(target)]}}
	var provider stage.Endpoint = h.terminal
	var err error
	switch target {
	case protocol.TypeOpenAIChat:
		provider, err = stage.Adapt(h.terminal, anthropicbridge.NewBetaToOpenAIChat(anthropicbridge.ChatOptions{}))
	case protocol.TypeOpenAIResponses:
		provider, err = stage.Adapt(h.terminal, anthropicbridge.NewBetaToOpenAIResponses(anthropicbridge.ResponsesOptions{}))
	}
	require.NoError(t, err)
	h.endpoint, err = stage.Compose(provider, toolround.New(toolround.Config{Owner: owner}))
	require.NoError(t, err)
	return h
}

// offeredRequest is the Beta request after tool injection: the client's own
// tool and the server's echo tool are both offered.
func offeredRequest(t *testing.T, messages string) *anthropic.BetaMessageNewParams {
	t.Helper()
	var request anthropic.BetaMessageNewParams
	require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(`{"model":"m","max_tokens":1024,
		"tools":[
			{"name":%q,"input_schema":{"type":"object","properties":{"location":{"type":"string"}}}},
			{"name":%q,"input_schema":{"type":"object","properties":{"q":{"type":"string"}}}}],
		"messages":%s}`, clientToolName, OwnedToolWireName, messages)), &request))
	return &request
}

const firstTurn = `[{"role":"user","content":"What is the capital of France?"}]`

// send runs one client request and returns what the client receives, in wire
// JSON (the message, or the concatenated stream events).
func (h *toolRoundHarness) send(ctx context.Context, t *testing.T, request *anthropic.BetaMessageNewParams, streaming bool) (string, error) {
	t.Helper()
	call := stage.Call{Request: request}
	if !streaming {
		response, err := h.endpoint.Complete(ctx, call)
		if err != nil {
			return "", err
		}
		return string(mustMarshal(response.Value)), nil
	}
	events, err := h.endpoint.Stream(ctx, call)
	if err != nil {
		return "", err
	}
	defer events.Close()
	var out strings.Builder
	for {
		event, err := events.Next(ctx)
		if errors.Is(err, io.EOF) {
			return out.String(), nil
		}
		if err != nil {
			return out.String(), err
		}
		data, err := wireJSON(event.Value)
		require.NoError(t, err)
		out.WriteString(data + "\n")
	}
}

func (h *toolRoundHarness) lastRequest() string {
	if len(h.terminal.requests) == 0 {
		return ""
	}
	return h.terminal.requests[len(h.terminal.requests)-1]
}

func forToolRoundTargets(t *testing.T, targets []protocol.APIType, f func(t *testing.T, target protocol.APIType, streaming bool)) {
	for _, target := range targets {
		for _, streaming := range []bool{false, true} {
			target, streaming := target, streaming
			t.Run(fmt.Sprintf("%s/stream=%v", target, streaming), func(t *testing.T) {
				t.Parallel()
				f(t, target, streaming)
			})
		}
	}
}

var allToolRoundTargets = []protocol.APIType{protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses}

func TestToolRoundOwnedLoopInMemory(t *testing.T) {
	forToolRoundTargets(t, allToolRoundTargets, func(t *testing.T, target protocol.APIType, streaming bool) {
		h := newToolRoundHarness(t, NewEchoServertoolProvider(), target, OwnedToolScenario())
		out, err := h.send(context.Background(), t, offeredRequest(t, firstTurn), streaming)
		require.NoError(t, err)
		require.Len(t, h.echo.Calls(), 1)
		require.Contains(t, h.lastRequest(), ownedToolResultText, "the tool result is fed back to the model")
		require.Contains(t, out, OwnedToolFinalText)
		require.NotContains(t, out, OwnedToolWireName, "the server tool never reaches the client")
	})
}

func TestToolRoundServerToolErrorInMemory(t *testing.T) {
	forToolRoundTargets(t, allToolRoundTargets, func(t *testing.T, target protocol.APIType, streaming bool) {
		h := newToolRoundHarness(t, &EchoServertoolProvider{Fail: true}, target, OwnedToolScenario())
		out, err := h.send(context.Background(), t, offeredRequest(t, firstTurn), streaming)
		require.NoError(t, err)
		require.Len(t, h.echo.Calls(), 1, "a failing tool is not retried")
		require.Contains(t, h.lastRequest(), ownedToolErrorText, "the failure is reported to the model")
		require.Contains(t, out, OwnedToolFinalText)
		require.NotContains(t, out, OwnedToolWireName)
	})
}

func TestToolRoundBoundedInMemory(t *testing.T) {
	targets := []protocol.APIType{protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat}
	forToolRoundTargets(t, targets, func(t *testing.T, target protocol.APIType, streaming bool) {
		h := newToolRoundHarness(t, NewEchoServertoolProvider(), target, AlwaysOwnedToolScenario())
		out, err := h.send(context.Background(), t, offeredRequest(t, firstTurn), streaming)
		require.NoError(t, err, "the round limit ends the turn, it does not fail it")
		require.Len(t, h.echo.Calls(), toolround.DefaultMaxRounds)
		require.Len(t, h.terminal.requests, toolround.DefaultMaxRounds+1)
		require.NotContains(t, out, OwnedToolWireName, "the cut-off never hands the server tool to the client")
	})
}

func TestToolRoundMixedContinuationInMemory(t *testing.T) {
	targets := []protocol.APIType{protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat}
	forToolRoundTargets(t, targets, func(t *testing.T, target protocol.APIType, streaming bool) {
		h := newToolRoundHarness(t, NewEchoServertoolProvider(), target, MixedToolScenario())
		ctx := typ.WithSessionID(context.Background(), typ.SessionID{Source: "header", Value: "mixed-" + t.Name()})

		first, err := h.send(ctx, t, offeredRequest(t, firstTurn), streaming)
		require.NoError(t, err)
		require.Len(t, h.echo.Calls(), 1)
		require.NotContains(t, first, OwnedToolWireName)
		clientID := ""
		for _, id := range []string{"toolu-client-tool", "call-client-tool"} {
			if strings.Contains(first, id) {
				clientID = id
			}
		}
		require.NotEmpty(t, clientID, "the client receives its own tool call:\n%s", first)

		followUp := offeredRequest(t, fmt.Sprintf(`[
			{"role":"user","content":"What is the capital of France?"},
			{"role":"assistant","content":[{"type":"tool_use","id":%q,"name":%q,"input":{"location":"Paris"}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":%q,"content":%q}]}]`, clientID, clientToolName, clientID, clientToolResultText))
		second, err := h.send(ctx, t, followUp, streaming)
		require.NoError(t, err)
		require.Contains(t, h.lastRequest(), ownedToolResultText, "the stored server-tool result is spliced into the follow-up")
		require.Contains(t, second, OwnedToolFinalText)
		require.Len(t, h.echo.Calls(), 1, "the server tool ran once in total")
	})
}
