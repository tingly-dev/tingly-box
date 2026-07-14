package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	protocolstage "github.com/tingly-dev/tingly-box/internal/protocol/stage"
	stagetoolloop "github.com/tingly-dev/tingly-box/internal/protocol/stage/toolloop"
)

// AnthropicBetaToolProvider prepares one Beta request with the exact tools the
// Stage owns. Returning ownership explicitly avoids treating a client-declared
// tool as internal merely because its name resembles an MCP tool name.
type AnthropicBetaToolProvider interface {
	PrepareRequest(ctx context.Context, request *anthropic.BetaMessageNewParams) ([]string, error)
}

// AnthropicBetaStageExecutor is the existing server-tool execution boundary
// needed by the Beta-native Stage. The call remains a Beta tool_use until this
// boundary; no canonical tool DTO is introduced.
type AnthropicBetaStageExecutor interface {
	ExecuteToolWithContext(ctx context.Context, tool Tool, messages []map[string]any) (context.Context, ToolExecutionResult, error)
}

type AnthropicBetaStageConfig struct {
	Name      string
	Tools     AnthropicBetaToolProvider
	Executor  AnthropicBetaStageExecutor
	MaxRounds int
}

func NewAnthropicBetaStage(config AnthropicBetaStageConfig) (protocolstage.Stage, error) {
	if config.Tools == nil {
		return nil, errors.New("construct Anthropic Beta ToolLoop Stage: tool provider is nil")
	}
	if config.Executor == nil {
		return nil, errors.New("construct Anthropic Beta ToolLoop Stage: executor is nil")
	}
	name := config.Name
	if name == "" {
		name = "tool_loop_anthropic_beta"
	}
	maxRounds := config.MaxRounds
	if maxRounds <= 0 {
		maxRounds = defaultMaxRounds
	}
	return &anthropicBetaToolLoopStage{
		name:      name,
		tools:     config.Tools,
		executor:  config.Executor,
		maxRounds: maxRounds,
		adapter:   NewAnthropicBetaAdapter(),
	}, nil
}

type anthropicBetaToolLoopStage struct {
	name      string
	tools     AnthropicBetaToolProvider
	executor  AnthropicBetaStageExecutor
	maxRounds int
	adapter   *AnthropicBetaAdapter
}

func (s *anthropicBetaToolLoopStage) Name() string             { return s.name }
func (*anthropicBetaToolLoopStage) Protocol() protocol.APIType { return protocol.TypeAnthropicBeta }
func (s *anthropicBetaToolLoopStage) Wrap(next protocolstage.Endpoint) protocolstage.Endpoint {
	return &anthropicBetaToolLoopEndpoint{stage: s, next: next}
}

type anthropicBetaToolLoopEndpoint struct {
	stage *anthropicBetaToolLoopStage
	next  protocolstage.Endpoint
}

func (*anthropicBetaToolLoopEndpoint) Protocol() protocol.APIType {
	return protocol.TypeAnthropicBeta
}

func (e *anthropicBetaToolLoopEndpoint) Complete(ctx context.Context, call protocolstage.Call) (*protocolstage.Response, error) {
	prepared, owned, err := e.prepare(ctx, call)
	if err != nil {
		return nil, err
	}

	runCtx := ctx
	current := prepared
	var totalUsage *protocol.TokenUsage
	sideEffectsCommitted := false
	for round := 1; round <= e.stage.maxRounds; round++ {
		response, callErr := e.next.Complete(runCtx, current)
		if callErr != nil {
			return nil, stagetoolloop.WrapError(callErr, sideEffectsCommitted)
		}
		if response == nil {
			return nil, stagetoolloop.WrapError(errors.New("Anthropic Beta ToolLoop received a nil response"), sideEffectsCommitted)
		}
		totalUsage = mergeBetaStageUsage(totalUsage, response.Usage)
		sideEffectsCommitted = sideEffectsCommitted || response.SideEffectsCommitted

		message, parseErr := betaStageMessage(response.Value)
		if parseErr != nil {
			return nil, stagetoolloop.WrapError(parseErr, sideEffectsCommitted)
		}
		tools, extractErr := e.stage.adapter.ExtractTools(message)
		if extractErr != nil {
			return nil, stagetoolloop.WrapError(extractErr, sideEffectsCommitted)
		}
		if !allBetaStageToolsOwned(tools, owned) {
			response.Usage = totalUsage
			response.SideEffectsCommitted = sideEffectsCommitted
			return response, nil
		}
		if round == e.stage.maxRounds {
			return nil, stagetoolloop.WrapError(stagetoolloop.ErrMaxRounds, sideEffectsCommitted)
		}

		results, nextCtx, committed := e.executeTools(runCtx, current.Request, tools)
		sideEffectsCommitted = sideEffectsCommitted || committed
		runCtx = nextCtx
		resultValues := make([]any, len(results))
		for i := range results {
			resultValues[i] = results[i]
		}
		nextRequest, appendErr := e.stage.adapter.AppendToolResults(current.Request, message, resultValues)
		if appendErr != nil {
			return nil, stagetoolloop.WrapError(appendErr, sideEffectsCommitted)
		}
		current.Request = nextRequest
	}
	return nil, stagetoolloop.WrapError(stagetoolloop.ErrMaxRounds, sideEffectsCommitted)
}

func (*anthropicBetaToolLoopEndpoint) Stream(context.Context, protocolstage.Call) (protocolstage.EventStream, error) {
	return nil, errors.New("Anthropic Beta ToolLoop streaming is not implemented")
}

func (e *anthropicBetaToolLoopEndpoint) prepare(ctx context.Context, call protocolstage.Call) (protocolstage.Call, map[string]struct{}, error) {
	request, ok := call.Request.(*anthropic.BetaMessageNewParams)
	if !ok || request == nil {
		return protocolstage.Call{}, nil, fmt.Errorf("Anthropic Beta ToolLoop received request %T", call.Request)
	}
	cloned, err := cloneBetaStageRequest(request)
	if err != nil {
		return protocolstage.Call{}, nil, err
	}
	existing := betaStageToolNames(cloned.Tools)
	ownedNames, err := e.stage.tools.PrepareRequest(ctx, cloned)
	if err != nil {
		return protocolstage.Call{}, nil, fmt.Errorf("prepare Anthropic Beta ToolLoop tools: %w", err)
	}
	owned := make(map[string]struct{}, len(ownedNames))
	for _, name := range ownedNames {
		if name == "" {
			return protocolstage.Call{}, nil, errors.New("Anthropic Beta ToolLoop provider returned an empty owned tool name")
		}
		if _, duplicated := owned[name]; duplicated {
			return protocolstage.Call{}, nil, fmt.Errorf("Anthropic Beta ToolLoop provider returned duplicate owned tool %q", name)
		}
		if _, collision := existing[name]; collision {
			return protocolstage.Call{}, nil, fmt.Errorf("%w: %q", stagetoolloop.ErrToolNameCollision, name)
		}
		owned[name] = struct{}{}
	}
	preparedNames := betaStageToolNames(cloned.Tools)
	for name := range owned {
		if _, injected := preparedNames[name]; !injected {
			return protocolstage.Call{}, nil, fmt.Errorf("Anthropic Beta ToolLoop provider claimed tool %q without injecting it", name)
		}
	}
	prepared := call
	prepared.Request = cloned
	return prepared, owned, nil
}

func (e *anthropicBetaToolLoopEndpoint) executeTools(
	ctx context.Context,
	request any,
	tools []Tool,
) ([]ToolExecutionResult, context.Context, bool) {
	messages := extractMessagesForToolCall(request)
	results := make([]ToolExecutionResult, 0, len(tools))
	runCtx := ctx
	committed := false
	for _, tool := range tools {
		nextCtx, result, err := e.stage.executor.ExecuteToolWithContext(runCtx, tool, messages)
		if nextCtx != nil {
			runCtx = nextCtx
		}
		if result.ToolUseID == "" {
			result.ToolUseID = tool.ID()
		}
		if err != nil {
			result.IsError = true
		} else {
			committed = true
		}
		results = append(results, result)
	}
	return results, runCtx, committed
}

func betaStageMessage(value any) (*anthropic.BetaMessage, error) {
	switch message := value.(type) {
	case *anthropic.BetaMessage:
		if message == nil {
			return nil, errors.New("Anthropic Beta ToolLoop received a nil message")
		}
		return message, nil
	case anthropic.BetaMessage:
		return &message, nil
	default:
		return nil, fmt.Errorf("Anthropic Beta ToolLoop received response %T", value)
	}
}

func allBetaStageToolsOwned(tools []Tool, owned map[string]struct{}) bool {
	if len(tools) == 0 {
		return false
	}
	for _, tool := range tools {
		if _, ok := owned[tool.Name()]; !ok {
			return false
		}
	}
	return true
}

func betaStageToolNames(tools []anthropic.BetaToolUnionParam) map[string]struct{} {
	names := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		if tool.OfTool != nil && tool.OfTool.Name != "" {
			names[tool.OfTool.Name] = struct{}{}
		}
	}
	return names
}

func cloneBetaStageRequest(request *anthropic.BetaMessageNewParams) (*anthropic.BetaMessageNewParams, error) {
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("clone Anthropic Beta ToolLoop request: %w", err)
	}
	var cloned anthropic.BetaMessageNewParams
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return nil, fmt.Errorf("clone Anthropic Beta ToolLoop request: %w", err)
	}
	return &cloned, nil
}

func mergeBetaStageUsage(total, current *protocol.TokenUsage) *protocol.TokenUsage {
	if current == nil {
		return total
	}
	if total == nil {
		copy := *current
		return &copy
	}
	total.InputTokens += current.InputTokens
	total.OutputTokens += current.OutputTokens
	total.CacheInputTokens += current.CacheInputTokens
	total.CacheReadTokens += current.CacheReadTokens
	total.CacheWriteTokens += current.CacheWriteTokens
	total.ReasoningTokens += current.ReasoningTokens
	total.SystemTokens += current.SystemTokens
	return total
}
