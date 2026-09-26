package toolengine

import (
	"context"
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol/stage/toolround"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AnthropicBetaOwner is the MCP side of the Tool Round Stage: it owns the
// server's virtual tools, runs them through the same ToolExecutor as the
// existing tool loops, and keeps mixed-round results in the same continuation store.
type AnthropicBetaOwner struct {
	registry     *coretool.VirtualToolRegistry
	executor     ToolExecutor
	providerUUID string
}

var _ toolround.Owner = (*AnthropicBetaOwner)(nil)

// NewAnthropicBetaOwner returns the owner for one provider attempt. The
// provider scopes stored continuations, as in the existing tool loops.
func NewAnthropicBetaOwner(registry *coretool.VirtualToolRegistry, executor ToolExecutor, providerUUID string) *AnthropicBetaOwner {
	return &AnthropicBetaOwner{registry: registry, executor: executor, providerUUID: providerUUID}
}

func (o *AnthropicBetaOwner) Owns(name string) bool {
	return IsVirtualTool(name, o.registry)
}

func (o *AnthropicBetaOwner) Execute(ctx context.Context, call toolround.ToolCall, request *anthropic.BetaMessageNewParams) (context.Context, anthropic.BetaToolResultBlockParam) {
	if o.executor == nil {
		// As the existing tool loops: the model gets an error result.
		return ctx, anthropic.BetaToolResultBlockParam{
			ToolUseID: call.ID,
			Content:   toolContentsToAnthropicBeta([]coretool.ToolContent{{Type: coretool.ContentTypeText, Text: "tool executor is not configured"}}),
			IsError:   anthropic.Bool(true),
		}
	}
	tool := ownedToolCall{call}
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
	// Only the client's follow-up to the stored round (it answers one of the
	// round's client calls) resumes it; any other request leaves it stored.
	segment, ok := mixedContinuationStore.popAnswered(o.continuationKey(ctx), request)
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
	key := o.continuationKey(ctx)
	if key == "" {
		logrus.Warn("[MCP-CONT] mixed round without a session: server tool results cannot be resumed")
		return
	}
	mixedContinuationStore.put(key, []anthropic.BetaMessageParam{turn, anthropic.NewBetaUserMessage(blocks...)})
}

// continuationKey scopes stored rounds to the session and provider, with the
// existing tool loops' key format; it is empty without a session.
func (o *AnthropicBetaOwner) continuationKey(ctx context.Context) string {
	return continuationKey(typ.GetSessionID(ctx), o.providerUUID, "anthropic-beta")
}

// ownedToolCall presents a Tool Round call as the executor's Tool.
type ownedToolCall struct{ call toolround.ToolCall }

func (t ownedToolCall) ID() string   { return t.call.ID }
func (t ownedToolCall) Name() string { return t.call.Name }

func (t ownedToolCall) Arguments() string {
	b, err := json.Marshal(t.call.Input)
	if err != nil || len(b) == 0 {
		return "{}"
	}
	return string(b)
}
