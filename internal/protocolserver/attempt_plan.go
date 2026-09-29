package protocolserver

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// attemptPlan is what one failover attempt resolves from its candidate before
// the request is touched: where the request goes and which rule-driven steps
// run on it. It is phase ① of .design/protocol-stage-pipeline.md, shared by
// the four run*Attempt entry points so none of them resolves the target or the
// flags on its own.
type attemptPlan struct {
	// Source is the client's protocol; Target the provider's, after dual-endpoint
	// resolution and OpenAI endpoint routing.
	Source, Target protocol.APIType
	// Provider is the candidate with its dual endpoint resolved for Source.
	Provider *typ.Provider
	// Model is the model sent upstream.
	Model string

	// Flags are the rule flags with scenario inheritance applied; PreBase and
	// PreVendor are the rule transforms they select for the two chain slots.
	Flags     typ.RuleFlags
	PreBase   []transform.Transform
	PreVendor []transform.Transform

	// MaxAllowed is the model's output-token limit on this provider, applied
	// to the upstream-bound request (OutputLimitTransform).
	MaxAllowed int
	// DefaultMaxTokens fills an Anthropic client's missing max_tokens
	// (MaxTokensDefaultTransform).
	DefaultMaxTokens int
}

// planAttempt resolves the attempt plan for one candidate. It stores the
// resolved provider on the gin context and, through
// ResolveRuleFlagsWithScenario, attaches the flags (and the client's
// User-Agent) to the request context for the outbound client layer.
func (ph *ProtocolHandler) planAttempt(c *gin.Context, rule *typ.Rule, provider *typ.Provider, model string, source protocol.APIType, scenarioType typ.RuleScenario, scenarioConfig *typ.ScenarioConfig) (*attemptPlan, error) {
	// Resolve dual endpoint: when the provider has a URL in the client's own
	// style configured, route there natively to avoid a conversion.
	provider = provider.ResolveStyle(clientAPIStyle(source))
	c.Set(ContextKeyProvider, provider)
	if provider.Timeout <= 0 {
		provider.Timeout = constant.DefaultRequestTimeout
	}

	target, err := ph.resolveAttemptTarget(c, rule, provider, model, source)
	if err != nil {
		return nil, err
	}
	flags := ResolveRuleFlagsWithScenario(c, rule, scenarioType, scenarioConfig, source, target, provider)
	return &attemptPlan{
		Source:           source,
		Target:           target,
		Provider:         provider,
		Model:            model,
		Flags:            flags,
		PreBase:          RulePreBaseTransforms(flags),
		PreVendor:        RulePreVendorTransforms(flags),
		MaxAllowed:       ph.deps.TemplateManager.GetMaxTokensForModelByProvider(provider, model),
		DefaultMaxTokens: ph.deps.Config.GetDefaultMaxTokens(),
	}, nil
}

// resolveAttemptTarget maps the provider's API style to the protocol the
// request is sent in. An OpenAI-style provider goes through endpoint routing
// (.design/openai-endpoint-routing.md): Chat clients prefer Chat, every other
// client prefers Responses.
func (ph *ProtocolHandler) resolveAttemptTarget(c *gin.Context, rule *typ.Rule, provider *typ.Provider, model string, source protocol.APIType) (protocol.APIType, error) {
	anthropicClient := source == protocol.TypeAnthropicV1 || source == protocol.TypeAnthropicBeta
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		if anthropicClient {
			return source, nil
		}
		return protocol.TypeAnthropicBeta, nil
	case protocol.APIStyleGoogle:
		if source == protocol.TypeOpenAIResponses {
			return "", fmt.Errorf("Responses API does not support Google-style providers yet. Provider: %s", provider.Name)
		}
		return protocol.TypeGoogle, nil
	case protocol.APIStyleOpenAI:
		incoming := IncomingAPIResponses
		if source == protocol.TypeOpenAIChat {
			incoming = IncomingAPIChat
		}
		modelOverride := ph.deps.TemplateManager.GetOpenAIEndpointOverrideForModel(provider, model)
		return ResolveOpenAIEndpoint(provider, ResolveRuleFlags(c, rule), incoming, modelOverride)
	}
	if anthropicClient {
		// Anthropic clients have always fallen back to their own protocol for
		// an unrecognized style.
		return source, nil
	}
	return "", fmt.Errorf("Unsupported provider API style: %s %s", provider.Name, provider.APIStyle)
}

// servedByStage reports whether this attempt goes through the Protocol Stage
// pipeline (serveAnthropicOnOpenAI / serveOpenAIOnAnthropic) rather than the
// full transform chain.
func (p *attemptPlan) servedByStage() bool {
	switch p.Source {
	case protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta:
		return p.Target == protocol.TypeOpenAIChat || p.Target == protocol.TypeOpenAIResponses
	case protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses:
		return p.Target == protocol.TypeAnthropicBeta
	}
	return false
}

// clientAPIStyle is the provider API style that serves a client protocol
// without conversion.
func clientAPIStyle(source protocol.APIType) protocol.APIStyle {
	switch source {
	case protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta:
		return protocol.APIStyleAnthropic
	case protocol.TypeGoogle:
		return protocol.APIStyleGoogle
	}
	return protocol.APIStyleOpenAI
}
