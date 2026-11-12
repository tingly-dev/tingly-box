package protocolserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/catalog"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/forwarding"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/upstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/translation"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func (ph *ProtocolHandler) translationBackend(c *gin.Context, rule *typ.Rule, provider *typ.Provider, model string) translation.Translator {
	if provider == nil || !provider.Enabled || provider.IsOAuth() || provider.IsVirtual() || provider.IsMultiFieldCredential() {
		return nil
	}
	switch provider.APIStyle {
	case protocol.APIStyleLibreTranslate, protocol.APIStyleDeepL:
		return client.NewTranslationClient(provider, typ.GetSessionID(c.Request.Context()))
	}
	provider = provider.ResolveStyle(protocol.APIStyleOpenAI)
	if model == "" || (provider.APIStyle != protocol.APIStyleOpenAI && provider.APIStyle != protocol.APIStyleAnthropic) {
		return nil
	}
	kind := catalog.TranslationAdapter(model)
	if kind != "" && (provider.APIStyle != protocol.APIStyleOpenAI || provider.OpenAIEndpointMode == "responses") {
		return nil
	}
	if kind == "" {
		kind = translation.LLM
	}
	return translation.ModelTranslator{Kind: kind, Model: model, Complete: func(ctx context.Context, req *openai.ChatCompletionNewParams) (*openai.ChatCompletion, *protocol.TokenUsage, error) {
		return ph.completeTranslationModel(ctx, c, rule, provider, model, kind, req)
	}}
}

// Generic LLM translation reuses the normal attempt planner, transforms,
// protocol conversion and forwarding. Native MT skips generic chat transforms
// because its user-only/template/translation_options contract is different.
func (ph *ProtocolHandler) completeTranslationModel(ctx context.Context, c *gin.Context, rule *typ.Rule, provider *typ.Provider, model, kind string, req *openai.ChatCompletionNewParams) (*openai.ChatCompletion, *protocol.TokenUsage, error) {
	if kind != translation.LLM {
		resp, cancel, err := forwarding.ForwardOpenAIChat(forwarding.NewForwardContext(ctx, provider), ph.deps.ClientPool.GetOpenAIClient(ctx, provider, model), req)
		if cancel != nil {
			defer cancel()
		}
		return resp, translationCompletionUsage(resp), err
	}
	scenario := rule.GetScenario()
	plan, err := ph.planAttempt(c, rule, provider, model, protocol.TypeOpenAIChat, scenario, ph.deps.Config.GetScenarioConfig(scenario))
	if err != nil {
		return nil, nil, err
	}
	source, err := ph.TransformOpenAIChat(c, &protocol.OpenAIChatCompletionRequest{ChatCompletionNewParams: req}, plan, false, scenario)
	if err != nil {
		return nil, nil, err
	}
	defer source.Release()
	provider = plan.Provider
	fc := forwarding.NewForwardContext(ctx, provider)
	switch plan.Target {
	case protocol.TypeOpenAIChat:
		resp, cancel, err := forwarding.ForwardOpenAIChat(fc, ph.deps.ClientPool.GetOpenAIClient(ctx, provider, model), source.Request.(*openai.ChatCompletionNewParams))
		if cancel != nil {
			defer cancel()
		}
		return resp, translationCompletionUsage(resp), err
	case protocol.TypeOpenAIResponses:
		resp, cancel, err := forwarding.ForwardOpenAIResponses(fc, ph.deps.ClientPool.GetOpenAIClient(ctx, provider, model), *source.Request.(*responses.ResponseNewParams))
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			return nil, nil, err
		}
		data, err := json.Marshal(nonstream.ConvertResponsesToOpenAIChat(resp, model))
		if err != nil {
			return nil, nil, err
		}
		var chat openai.ChatCompletion
		if err = json.Unmarshal(data, &chat); err != nil {
			return nil, nil, err
		}
		var usage *protocol.TokenUsage
		if resp.JSON.Usage.Valid() && resp.Usage.JSON.InputTokens.Valid() && resp.Usage.JSON.OutputTokens.Valid() {
			usage = protocol.NewTokenUsageWithCache(int(resp.Usage.InputTokens), int(resp.Usage.OutputTokens), int(resp.Usage.InputTokensDetails.CachedTokens))
		}
		return &chat, usage, nil
	case protocol.TypeAnthropicBeta:
		beta, err := openAIBetaRequest(source)
		if err != nil {
			return nil, nil, err
		}
		terminal, err := upstream.NewAnthropic(upstream.Config{Clients: ph.deps.ClientPool, Provider: provider, Model: model}, upstream.AnthropicWireBeta)
		if err != nil {
			return nil, nil, err
		}
		endpoint, err := stage.Compose(terminal, newTargetTransformStage(plan.Target, source, ph.targetTransforms(c, plan)))
		if err != nil {
			return nil, nil, err
		}
		resp, err := endpoint.Complete(ctx, stage.Call{Request: beta})
		if err != nil {
			return nil, nil, err
		}
		message, ok := resp.Value.(*anthropic.BetaMessage)
		if !ok || message == nil {
			return nil, nil, fmt.Errorf("invalid model response")
		}
		data, err := json.Marshal(ConvertAnthropicToOpenAIResponseWithProvider(message, model, provider, model))
		if err != nil {
			return nil, nil, err
		}
		var chat openai.ChatCompletion
		if err = json.Unmarshal(data, &chat); err != nil {
			return nil, nil, err
		}
		var usage *protocol.TokenUsage
		if message.JSON.Usage.Valid() && message.Usage.JSON.InputTokens.Valid() && message.Usage.JSON.OutputTokens.Valid() {
			usage = resp.Usage
		}
		return &chat, usage, nil
	}
	return nil, nil, fmt.Errorf("unsupported translation model transport")
}

func translationCompletionUsage(resp *openai.ChatCompletion) *protocol.TokenUsage {
	if resp == nil || !resp.JSON.Usage.Valid() || !resp.Usage.JSON.PromptTokens.Valid() || !resp.Usage.JSON.CompletionTokens.Valid() || resp.Usage.PromptTokens < 0 || resp.Usage.CompletionTokens < 0 {
		return nil
	}
	return protocol.NewTokenUsageWithCache(int(resp.Usage.PromptTokens), int(resp.Usage.CompletionTokens), int(resp.Usage.PromptTokensDetails.CachedTokens))
}
