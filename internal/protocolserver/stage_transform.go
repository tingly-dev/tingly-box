package protocolserver

import (
	"context"
	"fmt"
	"maps"

	"github.com/openai/openai-go/v3"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// targetTransformStage runs the upstream-bound half of the transform chain
// (targetTransforms: MCP tool injection, consistency, preVendor rules, vendor)
// on every request a pipeline sends to the provider, in the provider's
// protocol. The source half ran once on the client's request; a Bridge above
// this stage converted it, as BaseTransform does in the full chain.
//
// It runs per call because a Tool Round Stage above may send several rounds,
// each converted afresh; every round gets the same treatment the legacy chain
// gave the single converted request.
type targetTransformStage struct {
	target     protocol.APIType
	base       transform.TransformContext // request-scoped settings from the source half
	transforms []transform.Transform
}

func newTargetTransformStage(target protocol.APIType, source *transform.TransformContext, transforms []transform.Transform) *targetTransformStage {
	base := *source
	base.Request, base.OriginalRequest, base.TransformSteps = nil, nil, nil
	base.TargetAPI = target
	return &targetTransformStage{target: target, base: base, transforms: transforms}
}

func (s *targetTransformStage) Name() string { return "provider_transforms" }

func (s *targetTransformStage) Protocol() protocol.APIType { return s.target }

func (s *targetTransformStage) Wrap(next stage.Endpoint) stage.Endpoint {
	return &targetTransformEndpoint{stage: s, next: next}
}

type targetTransformEndpoint struct {
	stage *targetTransformStage
	next  stage.Endpoint
}

func (e *targetTransformEndpoint) Protocol() protocol.APIType { return e.stage.target }

func (e *targetTransformEndpoint) Complete(ctx context.Context, call stage.Call) (*stage.Response, error) {
	call, err := e.prepare(call)
	if err != nil {
		return nil, err
	}
	return e.next.Complete(ctx, call)
}

func (e *targetTransformEndpoint) Stream(ctx context.Context, call stage.Call) (stage.EventStream, error) {
	call, err := e.prepare(call)
	if err != nil {
		return nil, err
	}
	return e.next.Stream(ctx, call)
}

func (e *targetTransformEndpoint) prepare(call stage.Call) (stage.Call, error) {
	tctx := e.stage.base
	tctx.Request = call.Request
	tctx.OriginalRequest = call.Request
	tctx.Extra = maps.Clone(e.stage.base.Extra)
	if tctx.Extra == nil {
		tctx.Extra = map[string]interface{}{}
	}
	switch e.stage.target {
	case protocol.TypeOpenAIChat:
		if call.State.OpenAIChat != nil {
			tctx.Config.OpenAIConfig = call.State.OpenAIChat
		}
	case protocol.TypeOpenAIResponses:
		// As BaseTransform sets for an Anthropic request converted to Responses.
		tctx.Config.ResponsesConfig = &protocol.OpenAIConfig{HasThinking: false, ReasoningEffort: "none"}
	}
	final, err := transform.NewTransformChain(e.stage.transforms).Execute(&tctx)
	if err != nil {
		return call, err
	}
	if chat, ok := final.Request.(*openai.ChatCompletionNewParams); ok {
		// As dispatchOpenAIChat does after the chain.
		request.CleanupOpenaiFields(chat)
	}
	if final.Request == nil {
		return call, fmt.Errorf("provider transforms produced no request")
	}
	call.Request = final.Request
	return call, nil
}

func errUnexpectedRequest(value any) error {
	return fmt.Errorf("stage route: unexpected request type %T", value)
}
