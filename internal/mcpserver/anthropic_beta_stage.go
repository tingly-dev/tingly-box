package mcpserver

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

// AnthropicBetaContinuationStore owns the Beta-native continuation segment
// needed when one model response mixes internal and client-owned tool calls.
// A production implementation may bind one instance to a provider and derive
// the session key from ctx; the Stage never knows that storage key.
type AnthropicBetaContinuationStore interface {
	Pop(ctx context.Context, request *anthropic.BetaMessageNewParams) ([]anthropic.BetaMessageParam, bool)
	// Put stores a continuation segment that the next request may pop once its
	// external tool results are present. It reports failure so the Stage never
	// silently drops the internal tool context.
	Put(ctx context.Context, segment []anthropic.BetaMessageParam, externalIDs []string) error
	// CanStash reports whether a continuation can be persisted for ctx. The
	// Stage checks it before committing server-tool side effects for a round
	// that must be stashed.
	CanStash(ctx context.Context) bool
}

type AnthropicBetaStageConfig struct {
	Name          string
	Tools         AnthropicBetaToolProvider
	Executor      AnthropicBetaStageExecutor
	Continuations AnthropicBetaContinuationStore
	MaxRounds     int
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
		maxRounds = stagetoolloop.DefaultMaxRounds
	}
	return &anthropicBetaToolLoopStage{
		name:          name,
		tools:         config.Tools,
		executor:      config.Executor,
		continuations: config.Continuations,
		maxRounds:     maxRounds,
		adapter:       NewAnthropicBetaAdapter(),
	}, nil
}

type anthropicBetaToolLoopStage struct {
	name          string
	tools         AnthropicBetaToolProvider
	executor      AnthropicBetaStageExecutor
	continuations AnthropicBetaContinuationStore
	maxRounds     int
	adapter       *AnthropicBetaAdapter
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
	// internalMessages accumulates the assistant/tool-result turns of internal
	// (server-owned) rounds the client never saw. When the model later asks for
	// a client-owned tool, these messages are carried into the continuation so
	// the managed results still reach the final answer.
	var internalMessages []anthropic.BetaMessageParam
	sideEffectsCommitted := false
	// maxRounds bounds server-tool executions; one extra provider round is
	// allowed so the model can produce the final answer after the last one.
	for round := 1; round <= e.stage.maxRounds+1; round++ {
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
		managed, external, externalIDs := splitBetaStageTools(tools, owned)
		if len(managed) > 0 && round > e.stage.maxRounds {
			// The tool-execution budget is exhausted and the model still
			// requests server-owned tools; fail closed instead of executing
			// beyond the budget or leaking internal tool definitions.
			return nil, stagetoolloop.WrapError(stagetoolloop.ErrMaxRounds, sideEffectsCommitted)
		}
		if len(managed) == 0 {
			// A final answer (no tool calls) or a pure-external round with no
			// internal context needs no continuation.
			if len(external) == 0 || len(internalMessages) == 0 {
				response.Usage = totalUsage
				response.SideEffectsCommitted = sideEffectsCommitted
				return response, nil
			}
			// An external-only round after internal rounds: carry the internal
			// context to the client's next turn when the continuation can be
			// persisted. Without an explicit session the store cannot bind the
			// segment safely (IP addresses are not a conversation identity), so
			// deliver the external round as-is; guardrail blocking and terminal
			// responses still work, and no new side effects are committed here.
			if e.canPersistContinuation(runCtx) {
				segment := append(internalMessages, betaMessageToParamPreservingThinking(message))
				if stashErr := e.stashContinuation(runCtx, segment, externalIDs); stashErr != nil {
					return nil, stagetoolloop.WrapError(stashErr, sideEffectsCommitted)
				}
			}
			response.Usage = totalUsage
			response.SideEffectsCommitted = sideEffectsCommitted
			return response, nil
		}
		if len(external) > 0 {
			if e.stage.continuations == nil {
				response.Usage = totalUsage
				response.SideEffectsCommitted = sideEffectsCommitted
				return response, nil
			}
			if !e.stage.continuations.CanStash(runCtx) {
				return nil, stagetoolloop.WrapError(stagetoolloop.ErrContinuationUnavailable, sideEffectsCommitted)
			}
			results, nextCtx, committed := e.executeTools(runCtx, current.Request, managed)
			sideEffectsCommitted = sideEffectsCommitted || committed
			runCtx = nextCtx
			normalized, normalizeErr := validateAndNormalizeMixedStash(externalIDs, results)
			if normalizeErr != nil {
				return nil, stagetoolloop.WrapError(normalizeErr, sideEffectsCommitted)
			}
			segmentValue, segmentErr := e.stage.adapter.BuildContinuationSegment(message, normalized)
			if segmentErr != nil {
				return nil, stagetoolloop.WrapError(segmentErr, sideEffectsCommitted)
			}
			segment, ok := segmentValue.([]anthropic.BetaMessageParam)
			if !ok || len(segment) == 0 {
				return nil, stagetoolloop.WrapError(errors.New("Anthropic Beta ToolLoop built an empty mixed continuation"), sideEffectsCommitted)
			}
			// Prepend earlier internal rounds so results from previous managed
			// rounds survive the client's next turn alongside this round's.
			segment = append(internalMessages, segment...)
			if stashErr := e.stashContinuation(runCtx, segment, externalIDs); stashErr != nil {
				return nil, stagetoolloop.WrapError(stashErr, sideEffectsCommitted)
			}
			filtered, filterErr := e.stage.adapter.FilterVirtualTools(message, external)
			if filterErr != nil {
				return nil, stagetoolloop.WrapError(filterErr, sideEffectsCommitted)
			}
			response.Value = filtered
			response.Usage = totalUsage
			response.SideEffectsCommitted = sideEffectsCommitted
			return response, nil
		}

		results, nextCtx, committed := e.executeTools(runCtx, current.Request, managed)
		sideEffectsCommitted = sideEffectsCommitted || committed
		runCtx = nextCtx
		resultValues := make([]any, len(results))
		for i := range results {
			resultValues[i] = results[i]
		}
		internalMessages = appendBetaStageInternalRound(internalMessages, message, resultValues)
		nextRequest, appendErr := e.stage.adapter.AppendToolResults(current.Request, message, resultValues)
		if appendErr != nil {
			return nil, stagetoolloop.WrapError(appendErr, sideEffectsCommitted)
		}
		current.Request = nextRequest
	}
	return nil, stagetoolloop.WrapError(stagetoolloop.ErrMaxRounds, sideEffectsCommitted)
}

func (e *anthropicBetaToolLoopEndpoint) Stream(ctx context.Context, call protocolstage.Call) (protocolstage.EventStream, error) {
	prepared, owned, err := e.prepare(ctx, call)
	if err != nil {
		return nil, err
	}
	return newAnthropicBetaToolLoopStream(ctx, e, prepared, owned)
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
	if e.stage.continuations != nil {
		if segment, ok := e.stage.continuations.Pop(ctx, cloned); ok {
			continued, applyErr := e.stage.adapter.ApplyContinuation(cloned, segment)
			if applyErr != nil {
				return protocolstage.Call{}, nil, fmt.Errorf("apply Anthropic Beta ToolLoop continuation: %w", applyErr)
			}
			var continuedOK bool
			cloned, continuedOK = continued.(*anthropic.BetaMessageNewParams)
			if !continuedOK || cloned == nil {
				return protocolstage.Call{}, nil, fmt.Errorf("apply Anthropic Beta ToolLoop continuation returned %T", continued)
			}
		}
	}
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
		}
		if err == nil || result.Dispatched {
			committed = true
		}
		results = append(results, result)
	}
	return results, runCtx, committed
}

// canPersistContinuation reports whether a continuation segment can be stored
// for this context. A nil store (test-only fallback) or a store that cannot
// bind to the current session cannot persist one.
func (e *anthropicBetaToolLoopEndpoint) canPersistContinuation(ctx context.Context) bool {
	return e.stage.continuations != nil && e.stage.continuations.CanStash(ctx)
}

// stashContinuation stores a continuation segment and surfaces persistence
// failure as a Stage error so the internal tool context is never silently lost.
func (e *anthropicBetaToolLoopEndpoint) stashContinuation(ctx context.Context, segment []anthropic.BetaMessageParam, externalIDs []string) error {
	if e.stage.continuations == nil {
		return nil
	}
	if err := e.stage.continuations.Put(ctx, segment, externalIDs); err != nil {
		return fmt.Errorf("store Anthropic Beta ToolLoop continuation: %w", err)
	}
	return nil
}

// appendBetaStageInternalRound records one server-owned round in the messages
// the client has not seen, mirroring AppendToolResults so the in-loop request
// and any later continuation segment stay in sync.
func appendBetaStageInternalRound(messages []anthropic.BetaMessageParam, message *anthropic.BetaMessage, results []any) []anthropic.BetaMessageParam {
	messages = append(messages, betaMessageToParamPreservingThinking(message))
	return append(messages, anthropic.NewBetaUserMessage(betaStageToolResultBlocks(results)...))
}

// betaStageToolResultBlocks converts execution results into Beta tool_result
// content blocks shared by the adapter's append and continuation builders.
func betaStageToolResultBlocks(results []any) []anthropic.BetaContentBlockParamUnion {
	blocks := make([]anthropic.BetaContentBlockParamUnion, 0, len(results))
	for _, r := range results {
		tr, ok := r.(ToolExecutionResult)
		if !ok {
			continue
		}
		blocks = append(blocks, anthropic.BetaContentBlockParamUnion{
			OfToolResult: &anthropic.BetaToolResultBlockParam{
				ToolUseID: tr.ToolUseID,
				Content:   toolContentsToAnthropicBeta(tr.Contents),
				IsError:   anthropic.Bool(tr.IsError),
			},
		})
	}
	return blocks
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

func splitBetaStageTools(tools []Tool, owned map[string]struct{}) (managed, external []Tool, externalIDs []string) {
	managed = make([]Tool, 0, len(tools))
	external = make([]Tool, 0, len(tools))
	for _, tool := range tools {
		if _, ok := owned[tool.Name()]; ok {
			managed = append(managed, tool)
			continue
		}
		external = append(external, tool)
		externalIDs = append(externalIDs, tool.ID())
	}
	return managed, external, externalIDs
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
	total.CacheReadTokens += current.CacheReadTokens
	total.CacheWriteTokens += current.CacheWriteTokens
	total.ReasoningTokens += current.ReasoningTokens
	total.SystemTokens += current.SystemTokens
	return total
}
