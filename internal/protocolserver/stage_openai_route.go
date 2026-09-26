package protocolserver

import (
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/toolround"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/upstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// serveOpenAIOnAnthropic serves an OpenAI client (Chat or Responses) on an
// Anthropic provider through the Protocol Stage pipeline (cutover C3):
//
//	adapter -> [Tool Round Stage, Beta] -> provider transforms (Beta)
//	        -> upstream endpoint (Beta wire)
//
// source is the client's request after the source half of the transform
// chain. It is converted to Beta at this edge, as BaseTransform does, and the
// adapter converts the Beta answer back, so Beta - the provider's own
// protocol - is the only one the pipeline works in.
func (ph *ProtocolHandler) serveOpenAIOnAnthropic(c *gin.Context, source *transform.TransformContext, preVendor []transform.Transform, rule *typ.Rule, provider *typ.Provider, requestModel, responseModel string, isStreaming bool) {
	req, err := openAIBetaRequest(source)
	if err != nil {
		ph.FailAttemptSetup(c, err)
		return
	}
	if c.GetHeader("X-Tingly-Debug-Routing") == "1" {
		setProbeUpstreamHeaders(c, source, rule, provider)
	}

	terminal, err := upstream.NewAnthropic(upstream.Config{
		Clients:  ph.deps.ClientPool,
		Provider: provider,
		Model:    requestModel,
	}, upstream.AnthropicWireBeta)
	if err != nil {
		ph.FailAttemptSetup(c, err)
		return
	}
	target := protocol.TypeAnthropicBeta
	// OpenAI clients get no request guardrails before the chain, so the gate
	// screens the converted request itself.
	endpoint, err := stage.Compose(terminal,
		toolround.New(ph.toolRoundConfig(c, provider, requestModel, false)),
		newTargetTransformStage(target, source, ph.targetTransforms(c, target, preVendor)),
	)
	if err != nil {
		ph.FailAttemptSetup(c, err)
		return
	}

	disableStreamUsage := ShouldStripUsage(source.Extra)
	if source.ScenarioFlags != nil {
		disableStreamUsage = disableStreamUsage || source.ScenarioFlags.SkipUsage
	}
	ph.ServeStageOpenAI(c, endpoint, StageOpenAIAttempt{
		Client:             source.SourceAPI,
		Request:            req,
		Provider:           provider,
		ActualModel:        requestModel,
		ResponseModel:      responseModel,
		Streaming:          isStreaming,
		DisableStreamUsage: disableStreamUsage,
		StripUsage:         ShouldStripUsage(source.Extra),
	})
}

// openAIBetaRequest converts the client's OpenAI request to Beta, as
// BaseTransform does for an Anthropic Beta target.
func openAIBetaRequest(source *transform.TransformContext) (*anthropic.BetaMessageNewParams, error) {
	var req *anthropic.BetaMessageNewParams
	switch client := source.Request.(type) {
	case *openai.ChatCompletionNewParams:
		req = request.ConvertOpenAIToAnthropicRequest(client, 4096)
	case *responses.ResponseNewParams:
		req = request.ConvertOpenAIResponsesToAnthropicBetaRequest(*client, source.Config.MaxTokens)
	default:
		return nil, errUnexpectedRequest(source.Request)
	}
	if req == nil {
		return nil, fmt.Errorf("stage route: converting the %s request to Anthropic Beta returned nil", source.SourceAPI)
	}
	return req, nil
}
