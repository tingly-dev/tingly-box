// Package toolround is the Tool Round Stage: the single decision point for
// every tool call a model makes, on the Anthropic Beta protocol.
//
// For each round the model produces, every tool_use is decided in a fixed
// order (see .design/protocol-stage-v2.md §2.2):
//
//  1. Gate (Guardrails): a blocked call ends the turn. It is replaced by a
//     text block carrying the block message; no other call of that round is
//     executed or handed to the client.
//  2. Ownership (MCP): calls the server owns are executed and their results,
//     screened by the Gate, are fed back for another round. When a round
//     mixes owned and client calls, the owned calls run now and their results
//     are stored for the client's follow-up request.
//  3. Everything else passes to the client.
//
// Text and thinking stream to the client live; only tool_use blocks are held
// until their round is decided. A streamed answer that spans several rounds
// reaches the client as one message; a complete answer is the final round.
package toolround

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
)

// DefaultMaxRounds bounds how many rounds may execute owned tools in one
// request, matching the legacy loop.
const DefaultMaxRounds = 3

// ToolCall is one tool_use the model produced.
type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// Owner decides and runs server-owned tools (MCP).
type Owner interface {
	// Owns reports whether the server executes the named tool.
	Owns(name string) bool
	// Execute runs one owned call. Tool failures are reported in the result
	// (is_error), never as an aborted request. The returned context carries
	// state later executions in the same request depend on.
	Execute(ctx context.Context, call ToolCall, request *anthropic.BetaMessageNewParams) (context.Context, anthropic.BetaToolResultBlockParam)
	// Resume splices results stored by an earlier mixed round into request.
	Resume(ctx context.Context, request *anthropic.BetaMessageNewParams) *anthropic.BetaMessageNewParams
	// Suspend stores a mixed round (the assistant turn and the owned results)
	// for the client's follow-up request.
	Suspend(ctx context.Context, turn anthropic.BetaMessageParam, results []anthropic.BetaToolResultBlockParam)
}

// Verdict is the Gate's decision on one tool call.
type Verdict struct {
	Block   bool
	Message string
}

// Gate screens a turn (Guardrails).
type Gate interface {
	// Request screens the client's request before the first round.
	Request(ctx context.Context, request *anthropic.BetaMessageNewParams) error
	// ToolUse decides one tool call against the conversation so far.
	ToolUse(ctx context.Context, request *anthropic.BetaMessageNewParams, call ToolCall) Verdict
	// ToolResult screens an owned tool's result before the model sees it and
	// may rewrite it in place.
	ToolResult(ctx context.Context, request *anthropic.BetaMessageNewParams, call ToolCall, result *anthropic.BetaToolResultBlockParam)
	// Restore returns a tool input with alias tokens replaced by real values.
	Restore(input json.RawMessage) json.RawMessage
	// Response screens the final client-bound message of a complete answer and
	// may rewrite it in place.
	Response(ctx context.Context, request *anthropic.BetaMessageNewParams, message *anthropic.BetaMessage)
}

// Config configures the stage. A nil Owner owns no tools; a nil Gate allows
// everything. With neither, the stage is a pass-through.
type Config struct {
	Owner     Owner
	Gate      Gate
	MaxRounds int
}

// New returns the Tool Round Stage.
func New(config Config) stage.Stage {
	if config.MaxRounds <= 0 {
		config.MaxRounds = DefaultMaxRounds
	}
	return &toolRoundStage{config: config}
}

type toolRoundStage struct{ config Config }

func (*toolRoundStage) Name() string { return "tool_round" }

func (*toolRoundStage) Protocol() protocol.APIType { return protocol.TypeAnthropicBeta }

func (s *toolRoundStage) Wrap(next stage.Endpoint) stage.Endpoint {
	if s.config.Owner == nil && s.config.Gate == nil {
		return next
	}
	return &endpoint{config: s.config, next: next}
}

type endpoint struct {
	config Config
	next   stage.Endpoint
}

func (*endpoint) Protocol() protocol.APIType { return protocol.TypeAnthropicBeta }

// prepare screens the client request and splices a stored continuation,
// returning a request the stage may extend without touching the caller's.
func (e *endpoint) prepare(ctx context.Context, call stage.Call) (*anthropic.BetaMessageNewParams, error) {
	var request anthropic.BetaMessageNewParams
	switch value := call.Request.(type) {
	case *anthropic.BetaMessageNewParams:
		if value == nil {
			return nil, fmt.Errorf("tool round: nil request")
		}
		request = *value
	case anthropic.BetaMessageNewParams:
		request = value
	default:
		return nil, fmt.Errorf("tool round: request has type %T, want anthropic.BetaMessageNewParams", call.Request)
	}
	if e.config.Gate != nil {
		if err := e.config.Gate.Request(ctx, &request); err != nil {
			return nil, err
		}
	}
	if e.config.Owner != nil {
		if resumed := e.config.Owner.Resume(ctx, &request); resumed != nil {
			request = *resumed
		}
	}
	return &request, nil
}

// plan is the decision for one round.
type plan struct {
	// continueLoop: execute every call (all owned) and run another round.
	continueLoop bool
	// execute: owned calls to run now (the whole round, or a mixed round).
	execute []ToolCall
	// suspend: store the executed results for the client's follow-up.
	suspend bool
	// blocked maps a tool_use id to its block message.
	blocked map[string]string
	// keep is the set of tool_use ids handed to the client.
	keep map[string]bool
	// stopReason overrides the round's stop_reason when non-empty.
	stopReason string
}

func (e *endpoint) decide(ctx context.Context, request *anthropic.BetaMessageNewParams, calls []ToolCall, executedRounds int) plan {
	p := plan{blocked: map[string]string{}, keep: map[string]bool{}}
	if len(calls) == 0 {
		return p
	}
	if gate := e.config.Gate; gate != nil {
		for _, call := range calls {
			if verdict := gate.ToolUse(ctx, request, call); verdict.Block {
				p.blocked[call.ID] = verdict.Message
			}
		}
		if len(p.blocked) > 0 {
			p.stopReason = string(anthropic.BetaStopReasonEndTurn)
			return p
		}
	}

	var owned, client []ToolCall
	for _, call := range calls {
		if e.config.Owner != nil && e.config.Owner.Owns(call.Name) {
			owned = append(owned, call)
		} else {
			client = append(client, call)
		}
	}
	for _, call := range client {
		p.keep[call.ID] = true
	}
	switch {
	case len(owned) == 0:
	case executedRounds >= e.config.MaxRounds:
		// The round limit ends the turn without running the owned calls and
		// without ever exposing them.
		if len(client) == 0 {
			p.stopReason = string(anthropic.BetaStopReasonEndTurn)
		}
	case len(client) == 0:
		p.continueLoop = true
		p.execute = owned
	default:
		p.execute = owned
		p.suspend = true
	}
	return p
}

// execute runs the plan's owned calls in order, screening each result.
func (e *endpoint) execute(ctx context.Context, request *anthropic.BetaMessageNewParams, calls []ToolCall) (context.Context, []anthropic.BetaToolResultBlockParam) {
	results := make([]anthropic.BetaToolResultBlockParam, 0, len(calls))
	for _, call := range calls {
		if e.config.Gate != nil {
			call.Input = e.config.Gate.Restore(call.Input)
		}
		next, result := e.config.Owner.Execute(ctx, call, request)
		if next != nil {
			ctx = next
		}
		if result.ToolUseID == "" {
			result.ToolUseID = call.ID
		}
		if e.config.Gate != nil {
			e.config.Gate.ToolResult(ctx, request, call, &result)
		}
		results = append(results, result)
	}
	return ctx, results
}

// continued returns request extended with the assistant turn and its results.
func continued(request *anthropic.BetaMessageNewParams, turn anthropic.BetaMessageParam, results []anthropic.BetaToolResultBlockParam) *anthropic.BetaMessageNewParams {
	next := *request
	next.Messages = append(append(make([]anthropic.BetaMessageParam, 0, len(request.Messages)+2), request.Messages...), turn, resultMessage(results))
	return &next
}

func resultMessage(results []anthropic.BetaToolResultBlockParam) anthropic.BetaMessageParam {
	blocks := make([]anthropic.BetaContentBlockParamUnion, len(results))
	for i := range results {
		blocks[i] = anthropic.BetaContentBlockParamUnion{OfToolResult: &results[i]}
	}
	return anthropic.NewBetaUserMessage(blocks...)
}

// assistantTurn converts a round's message into the assistant turn of the
// next request through its wire JSON, keeping thinking signatures and
// provider fields that ToParam may drop.
func assistantTurn(message *anthropic.BetaMessage) anthropic.BetaMessageParam {
	if raw := message.RawJSON(); raw != "" {
		var param anthropic.BetaMessageParam
		if err := json.Unmarshal([]byte(raw), &param); err == nil {
			return param
		}
	}
	return message.ToParam()
}

func toolCalls(message *anthropic.BetaMessage) []ToolCall {
	var calls []ToolCall
	for _, block := range message.Content {
		if block.Type != "tool_use" {
			continue
		}
		calls = append(calls, ToolCall{ID: block.ID, Name: block.Name, Input: toolInput(block)})
	}
	return calls
}

func toolInput(block anthropic.BetaContentBlockUnion) json.RawMessage {
	if raw := block.RawJSON(); raw != "" {
		if input := gjson.Get(raw, "input"); input.Exists() {
			return json.RawMessage(input.Raw)
		}
	}
	data, err := json.Marshal(block.Input)
	if err != nil || len(data) == 0 || string(data) == "null" {
		return json.RawMessage("{}")
	}
	return data
}

// clientBlock renders one content block for the client under plan p, or
// reports false when the block is withheld.
func (e *endpoint) clientBlock(p plan, raw string) (string, bool) {
	if gjson.Get(raw, "type").String() != "tool_use" {
		return raw, true
	}
	id := gjson.Get(raw, "id").String()
	if message, ok := p.blocked[id]; ok {
		return textBlockJSON(message), true
	}
	if !p.keep[id] {
		return "", false
	}
	if e.config.Gate != nil {
		input := gjson.Get(raw, "input").Raw
		if restored := string(e.config.Gate.Restore(json.RawMessage(input))); restored != input && restored != "" {
			raw, _ = sjson.SetRaw(raw, "input", restored)
		}
	}
	return raw, true
}

// clientMessage applies plan p to a round's message for the client.
func (e *endpoint) clientMessage(message *anthropic.BetaMessage, p plan) (*anthropic.BetaMessage, error) {
	if len(p.blocked) == 0 && p.stopReason == "" && e.config.Gate == nil && len(p.keep) == len(toolCalls(message)) {
		return message, nil
	}
	raw := message.RawJSON()
	if raw == "" {
		data, err := json.Marshal(message)
		if err != nil {
			return nil, fmt.Errorf("tool round: marshal message: %w", err)
		}
		raw = string(data)
	}
	blocks := make([]string, 0, len(message.Content))
	for _, block := range gjson.Get(raw, "content").Array() {
		if out, ok := e.clientBlock(p, block.Raw); ok {
			blocks = append(blocks, out)
		}
	}
	raw, err := sjson.SetRaw(raw, "content", "["+strings.Join(blocks, ",")+"]")
	if err == nil && p.stopReason != "" {
		raw, err = sjson.Set(raw, "stop_reason", p.stopReason)
	}
	if err != nil {
		return nil, fmt.Errorf("tool round: rewrite message: %w", err)
	}
	var out anthropic.BetaMessage
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("tool round: rewrite message: %w", err)
	}
	return &out, nil
}

func textBlockJSON(text string) string {
	raw, _ := sjson.Set(`{"type":"text","text":""}`, "text", text)
	return raw
}

func (e *endpoint) Complete(ctx context.Context, call stage.Call) (*stage.Response, error) {
	request, err := e.prepare(ctx, call)
	if err != nil {
		return nil, err
	}
	var usage *protocol.TokenUsage
	committed := false
	executedRounds := 0
	for {
		response, err := e.next.Complete(ctx, stage.Call{Request: request, Metadata: call.Metadata, State: call.State})
		if err != nil {
			return nil, stage.WrapCommitted(err, committed)
		}
		committed = committed || response.SideEffectsCommitted
		usage = addUsage(usage, response.Usage)
		message, ok := response.Value.(*anthropic.BetaMessage)
		if !ok {
			return nil, stage.WrapCommitted(fmt.Errorf("tool round: response has type %T, want *anthropic.BetaMessage", response.Value), committed)
		}

		p := e.decide(ctx, request, toolCalls(message), executedRounds)
		if len(p.execute) > 0 {
			var results []anthropic.BetaToolResultBlockParam
			ctx, results = e.execute(ctx, request, p.execute)
			committed = true
			executedRounds++
			turn := assistantTurn(message)
			if p.continueLoop {
				request = continued(request, turn, results)
				continue
			}
			e.config.Owner.Suspend(ctx, turn, results)
		}

		out, err := e.clientMessage(message, p)
		if err != nil {
			return nil, stage.WrapCommitted(err, committed)
		}
		if e.config.Gate != nil {
			e.config.Gate.Response(ctx, request, out)
		}
		return &stage.Response{
			Value:                out,
			Usage:                usage,
			Model:                response.Model,
			SideEffectsCommitted: committed,
		}, nil
	}
}

func addUsage(total, round *protocol.TokenUsage) *protocol.TokenUsage {
	if round == nil {
		return total
	}
	if total == nil {
		copied := *round
		return &copied
	}
	sum := *total
	sum.InputTokens += round.InputTokens
	sum.OutputTokens += round.OutputTokens
	sum.CacheReadTokens += round.CacheReadTokens
	sum.CacheWriteTokens += round.CacheWriteTokens
	sum.ReasoningTokens += round.ReasoningTokens
	sum.SystemTokens += round.SystemTokens
	return &sum
}
