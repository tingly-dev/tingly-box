package toolengine

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol/stage/toolround"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AnthropicBetaOwner is the MCP side of the Tool Round Stage: it owns the
// server's virtual tools, runs them through the same ToolExecutor as the
// legacy loops, and keeps mixed-round results in the same continuation store.
type AnthropicBetaOwner struct {
	registry     *coretool.VirtualToolRegistry
	executor     ToolExecutor
	providerUUID string
}

var _ toolround.Owner = (*AnthropicBetaOwner)(nil)

// NewAnthropicBetaOwner returns the owner for one provider attempt. The
// provider scopes stored continuations, as in the legacy loops.
func NewAnthropicBetaOwner(registry *coretool.VirtualToolRegistry, executor ToolExecutor, providerUUID string) *AnthropicBetaOwner {
	return &AnthropicBetaOwner{registry: registry, executor: executor, providerUUID: providerUUID}
}

func (o *AnthropicBetaOwner) Owns(name string) bool {
	return IsVirtualTool(name, o.registry)
}

func (o *AnthropicBetaOwner) Execute(ctx context.Context, call toolround.ToolCall, request *anthropic.BetaMessageNewParams) (context.Context, anthropic.BetaToolResultBlockParam) {
	tool := &AnthropicBetaTool{ToolUseBlock: anthropic.BetaToolUseBlock{ID: call.ID, Name: call.Name, Input: call.Input}}
	next, result, err := o.executor.ExecuteToolWithContext(ctx, tool, extractMessagesForToolCall(request))
	if err != nil {
		logrus.WithError(err).Warnf("tool execution failed: %s", call.Name)
	}
	if result.ToolUseID == "" {
		result.ToolUseID = call.ID
	}
	return next, anthropic.BetaToolResultBlockParam{
		ToolUseID: result.ToolUseID,
		Content:   toolContentsToAnthropicBeta(result.Contents),
		IsError:   anthropic.Bool(result.IsError),
	}
}

func (o *AnthropicBetaOwner) Resume(ctx context.Context, request *anthropic.BetaMessageNewParams) *anthropic.BetaMessageNewParams {
	segment, ok := mixedContinuationStore.pop(o.continuationKey(ctx))
	if !ok {
		return request
	}
	messages, ok := segment.([]anthropic.BetaMessageParam)
	if !ok || len(messages) == 0 {
		return request
	}
	resumed := *request
	resumed.Messages = mergeAnthropicBetaContinuation(messages, request.Messages)
	return &resumed
}

func (o *AnthropicBetaOwner) Suspend(ctx context.Context, turn anthropic.BetaMessageParam, results []anthropic.BetaToolResultBlockParam) {
	blocks := make([]anthropic.BetaContentBlockParamUnion, 0, len(results))
	for i := range results {
		if results[i].ToolUseID != "" {
			blocks = append(blocks, anthropic.BetaContentBlockParamUnion{OfToolResult: &results[i]})
		}
	}
	if len(blocks) == 0 {
		logrus.Warn("[MCP-CONT] mixed round produced no tool results to store")
		return
	}
	mixedContinuationStore.put(o.continuationKey(ctx), []anthropic.BetaMessageParam{turn, anthropic.NewBetaUserMessage(blocks...)})
}

func (o *AnthropicBetaOwner) continuationKey(ctx context.Context) string {
	return continuationKey(typ.GetSessionID(ctx), o.providerUUID, "anthropic-beta")
}
