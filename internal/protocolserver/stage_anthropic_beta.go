package protocolserver

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailspipeline "github.com/tingly-dev/tingly-box/internal/guardrails/pipeline"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/toolround"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/upstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/recording"
	mcp "github.com/tingly-dev/tingly-box/internal/toolengine"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// serveAnthropicBetaStage serves an Anthropic Beta client on an Anthropic
// provider through the Protocol Stage pipeline (cutover C1): the Tool Round
// Stage decides every tool call - Guardrails first, then MCP ownership - when
// either is on for the request, and is left out otherwise, so a plain request
// reaches the provider exactly as before.
func (ph *ProtocolHandler) serveAnthropicBetaStage(c *gin.Context, reqCtx *transform.TransformContext, rule *typ.Rule, provider *typ.Provider, isStreaming bool) {
	req := reqCtx.Request.(*anthropic.BetaMessageNewParams)

	terminal, err := upstream.NewAnthropic(upstream.Config{
		Clients:  ph.deps.ClientPool,
		Provider: provider,
		Model:    reqCtx.RequestModel,
	}, upstream.AnthropicWireBeta)
	if err != nil {
		ph.FailAttemptSetup(c, err)
		return
	}

	var config toolround.Config
	if ph.mcpEnabled() && ph.deps.MCPRuntime != nil {
		executor := mcp.NewServerToolExecutor(newServerOpsAdapter(ph, recording.FromGin(c)))
		config.Owner = mcp.NewAnthropicBetaOwner(ph.deps.MCPRuntime.VirtualRegistry(), executor, provider.UUID)
	}
	if ph.guardrailsEnabledForScenario(GetTrackingContextScenario(c)) {
		base := BuildGuardrailsBaseInput(c, reqCtx.RequestModel, provider, guardrailscore.DirectionResponse, nil)
		base.State.CredentialMask = EnsureGuardrailsCredentialMaskState(c)
		config.Gate = requestScreenedGate{guardrailspipeline.NewToolRoundGate(ph.currentGuardrailsRuntime(), base)}
	}
	endpoint, err := stage.Compose(terminal, toolround.New(config))
	if err != nil {
		ph.FailAttemptSetup(c, err)
		return
	}

	ph.ServeStageAnthropic(c, endpoint, StageAnthropicAttempt{
		Client:        protocol.TypeAnthropicBeta,
		Request:       req,
		Rule:          rule,
		Provider:      provider,
		ActualModel:   reqCtx.RequestModel,
		ResponseModel: reqCtx.ResponseModel,
		Streaming:     isStreaming,
	})
}

// requestScreenedGate is the Guardrails gate for a request whose client-side
// screening already ran before the transform chain
// (ApplyGuardrailsToAnthropicV1BetaRequest in runAnthropicBetaAttempt), with
// the same credential mask state, so the gate does not screen it again.
type requestScreenedGate struct {
	*guardrailspipeline.ToolRoundGate
}

func (requestScreenedGate) Request(context.Context, *anthropic.BetaMessageNewParams) error {
	return nil
}
