package protocolserver

import (
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

// serveAnthropicOnOpenAIChat serves an Anthropic client (V1 or Beta) on an
// OpenAI Chat provider through the Protocol Stage pipeline (cutover C2):
//
//	adapter -> [Tool Round Stage, Beta] -> Beta->Chat bridge
//	        -> provider transforms (Chat) -> upstream Chat endpoint
//
// source is the client's request after the source half of the transform
// chain; the bridge converts it as BaseTransform does, and the target half
// runs on every request sent upstream.
func (ph *ProtocolHandler) serveAnthropicOnOpenAIChat(c *gin.Context, source *transform.TransformContext, preVendor []transform.Transform, rule *typ.Rule, provider *typ.Provider, requestModel, responseModel string, isStreaming bool) {
	req, client, err := anthropicBetaRequest(source)
	if err != nil {
		ph.FailAttemptSetup(c, err)
		return
	}
	if c.GetHeader("X-Tingly-Debug-Routing") == "1" {
		setProbeUpstreamHeaders(c, source, rule, provider)
	}

	terminal, err := upstream.NewOpenAIChat(upstream.Config{Clients: ph.deps.ClientPool, Provider: provider, Model: requestModel})
	if err != nil {
		ph.FailAttemptSetup(c, err)
		return
	}
	chat, err := stage.Compose(terminal, newTargetTransformStage(protocol.TypeOpenAIChat, source, ph.targetTransforms(c, protocol.TypeOpenAIChat, preVendor)))
	if err != nil {
		ph.FailAttemptSetup(c, err)
		return
	}
	skipUsage := source.ScenarioFlags != nil && source.ScenarioFlags.SkipUsage
	beta, err := stage.Adapt(chat, anthropicbridge.NewBetaToOpenAIChat(anthropicbridge.ChatOptions{
		Compatible:         true, // as BaseTransform converts for Chat targets
		DisableStreamUsage: skipUsage,
		ResponseModel:      responseModel,
	}))
	if err != nil {
		ph.FailAttemptSetup(c, err)
		return
	}

	route := anthropicRoute{
		Client: client, Request: req, Rule: rule, Provider: provider,
		ActualModel: requestModel, ResponseModel: responseModel, Streaming: isStreaming,
	}
	if client == protocol.TypeAnthropicV1 {
		route.Finish = func(message *anthropic.BetaMessage) (*anthropic.BetaMessage, error) {
			if ShouldRoundtripResponse(c, "openai") {
				return RoundtripAnthropicBetaResponseViaOpenAI(message, responseModel, provider, requestModel)
			}
			return message, nil
		}
	}
	ph.serveAnthropicPipeline(c, route, beta)
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
