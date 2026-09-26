package protocolserver

import (
	"fmt"
	"maps"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/anthropicbridge"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/upstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// serveAnthropicOnOpenAI serves an Anthropic client (V1 or Beta) on an
// OpenAI Chat or Responses provider through the Protocol Stage pipeline
// (cutover C2):
//
//	adapter -> [Tool Round Stage, Beta] -> Beta->target bridge
//	        -> provider transforms (target) -> upstream endpoint
//
// source is the client's request after the source half of the transform
// chain; the bridge converts it as BaseTransform does, and the target half
// runs on every request sent upstream.
func (ph *ProtocolHandler) serveAnthropicOnOpenAI(c *gin.Context, target protocol.APIType, source *transform.TransformContext, preVendor []transform.Transform, rule *typ.Rule, provider *typ.Provider, requestModel, responseModel string, isStreaming bool) {
	req, client, err := anthropicBetaRequest(source)
	if err != nil {
		ph.FailAttemptSetup(c, err)
		return
	}
	if c.GetHeader("X-Tingly-Debug-Routing") == "1" {
		setProbeUpstreamHeaders(c, source, rule, provider)
	}

	config := upstream.Config{Clients: ph.deps.ClientPool, Provider: provider, Model: requestModel}
	var terminal stage.Endpoint
	var bridge stage.Bridge
	switch target {
	case protocol.TypeOpenAIChat:
		terminal, err = upstream.NewOpenAIChat(config)
		bridge = anthropicbridge.NewBetaToOpenAIChat(anthropicbridge.ChatOptions{
			Compatible:         true, // as BaseTransform converts for Chat targets
			DisableStreamUsage: source.ScenarioFlags != nil && source.ScenarioFlags.SkipUsage,
			ResponseModel:      responseModel,
		})
	case protocol.TypeOpenAIResponses:
		// The MCP transforms do not act on Responses requests, so server
		// tools are offered on the Beta request and converted with it.
		if req, err = ph.offerServerTools(source, req); err != nil {
			ph.FailAttemptSetup(c, err)
			return
		}
		config.StreamOnly = provider.IsCodexProvider()
		terminal, err = upstream.NewOpenAIResponses(config)
		bridge = anthropicbridge.NewBetaToOpenAIResponses(anthropicbridge.ResponsesOptions{ResponseModel: responseModel})
	default:
		err = fmt.Errorf("stage route: unsupported OpenAI target %s", target)
	}
	if err != nil {
		ph.FailAttemptSetup(c, err)
		return
	}
	providerSide, err := stage.Compose(terminal, newTargetTransformStage(target, source, ph.targetTransforms(c, target, preVendor)))
	if err != nil {
		ph.FailAttemptSetup(c, err)
		return
	}
	beta, err := stage.Adapt(providerSide, bridge)
	if err != nil {
		ph.FailAttemptSetup(c, err)
		return
	}

	route := anthropicRoute{
		Client: client, Request: req, Rule: rule, Provider: provider,
		ActualModel: requestModel, ResponseModel: responseModel, Streaming: isStreaming,
	}
	if client == protocol.TypeAnthropicV1 && target == protocol.TypeOpenAIChat {
		route.Finish = func(message *anthropic.BetaMessage) (*anthropic.BetaMessage, error) {
			if ShouldRoundtripResponse(c, "openai") {
				return RoundtripAnthropicBetaResponseViaOpenAI(message, responseModel, provider, requestModel)
			}
			return message, nil
		}
	}
	ph.serveAnthropicPipeline(c, route, beta)
}

// offerServerTools runs the MCP transforms (tool injection, native web
// search strip, strip guard) on a Beta request when MCP is on.
func (ph *ProtocolHandler) offerServerTools(source *transform.TransformContext, req *anthropic.BetaMessageNewParams) (*anthropic.BetaMessageNewParams, error) {
	if !ph.mcpEnabled() {
		return req, nil
	}
	tctx := *source
	tctx.Request, tctx.OriginalRequest, tctx.TransformSteps = req, req, nil
	tctx.Extra = maps.Clone(source.Extra)
	final, err := transform.NewTransformChain(ph.mcpChainTransforms(ph.mcpStripDisabledToolsEnabled())).Execute(&tctx)
	if err != nil {
		return nil, err
	}
	offered, ok := final.Request.(*anthropic.BetaMessageNewParams)
	if !ok {
		return nil, errUnexpectedRequest(final.Request)
	}
	return offered, nil
}

// anthropicBetaRequest returns the client's request as Beta, upgrading a V1
// request at this edge, and the client's protocol.
func anthropicBetaRequest(source *transform.TransformContext) (*anthropic.BetaMessageNewParams, protocol.APIType, error) {
	switch req := source.Request.(type) {
	case *anthropic.BetaMessageNewParams:
		return req, protocol.TypeAnthropicBeta, nil
	case *anthropic.MessageNewParams:
		beta, err := request.ConvertAnthropicV1ToBetaRequestWithError(req)
		return beta, protocol.TypeAnthropicV1, err
	}
	return nil, "", errUnexpectedRequest(source.Request)
}
