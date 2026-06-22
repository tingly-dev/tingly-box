package transform

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	protocoltransform "github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// MaxTokensTransform ensures max_tokens is set and properly bounded for all supported protocols.
//
// It applies three rules consistently across protocols:
//  1. Fill defaultMaxTokens when the client sends 0 or omits the field
//  2. Cap max_tokens at maxAllowed (model's maximum)
//  3. For Anthropic: handle thinking budget capping
//
// Supported protocols:
// - Anthropic V1 (Message API)
// - Anthropic Beta (extended Message API)
// - OpenAI Chat (max_tokens and max_completion_tokens)
// - OpenAI Responses (max_output_tokens)
type MaxTokensTransform struct {
	DefaultMaxTokens int64
	MaxAllowed       int64
}

// NewMaxTokensTransform creates a MaxTokensTransform with the given configuration.
// These values typically come from:
// - DefaultMaxTokens: global config (Anthropic) or template defaults (OpenAI)
// - MaxAllowed: template manager or rule-level override
func NewMaxTokensTransform(defaultMaxTokens, maxAllowed int64) *MaxTokensTransform {
	return &MaxTokensTransform{
		DefaultMaxTokens: defaultMaxTokens,
		MaxAllowed:       maxAllowed,
	}
}

func (t *MaxTokensTransform) Name() string { return "max_tokens" }

func (t *MaxTokensTransform) Apply(ctx *protocoltransform.TransformContext) error {
	switch req := ctx.Request.(type) {
	case *anthropic.MessageNewParams:
		t.applyAnthropicV1(req)
	case *anthropic.BetaMessageNewParams:
		t.applyAnthropicBeta(req)
	case *openai.ChatCompletionNewParams:
		t.applyOpenAIChat(req)
	case *responses.ResponseNewParams:
		t.applyOpenAIResponses(req)
	default:
		// No-op for unsupported protocols (Google, etc.)
	}
	return nil
}

// applyAnthropicV1 handles max_tokens for Anthropic V1 Message API
func (t *MaxTokensTransform) applyAnthropicV1(req *anthropic.MessageNewParams) {
	if req.MaxTokens == 0 {
		req.MaxTokens = t.DefaultMaxTokens
	}
	if req.MaxTokens > t.MaxAllowed {
		req.MaxTokens = t.MaxAllowed
	}
	if thinkBudget := req.Thinking.GetBudgetTokens(); thinkBudget != nil {
		if *thinkBudget > t.MaxAllowed {
			req.Thinking = anthropic.ThinkingConfigParamOfEnabled(max(1024, t.MaxAllowed/10))
		}
		// Anthropic enforces budget_tokens <= max_tokens. max_tokens is a hard
		// operator limit — cap the budget rather than raising the limit.
		if budget := req.Thinking.GetBudgetTokens(); budget != nil && *budget > req.MaxTokens {
			req.Thinking = anthropic.ThinkingConfigParamOfEnabled(max(1024, req.MaxTokens))
		}
	}
}

// applyAnthropicBeta handles max_tokens for Anthropic Beta Message API
func (t *MaxTokensTransform) applyAnthropicBeta(req *anthropic.BetaMessageNewParams) {
	if req.MaxTokens == 0 {
		req.MaxTokens = t.DefaultMaxTokens
	}
	if req.MaxTokens > t.MaxAllowed {
		req.MaxTokens = t.MaxAllowed
	}
	if thinkBudget := req.Thinking.GetBudgetTokens(); thinkBudget != nil {
		if *thinkBudget > t.MaxAllowed {
			req.Thinking = anthropic.BetaThinkingConfigParamOfEnabled(max(1024, t.MaxAllowed/10))
		}
		// Anthropic enforces budget_tokens <= max_tokens. max_tokens is a hard
		// operator limit — cap the budget rather than raising the limit.
		if budget := req.Thinking.GetBudgetTokens(); budget != nil && *budget > req.MaxTokens {
			req.Thinking = anthropic.BetaThinkingConfigParamOfEnabled(max(1024, req.MaxTokens))
		}
	}
}

// applyOpenAIChat handles max_tokens and max_completion_tokens for OpenAI Chat API.
//
// OpenAI has two token limit fields:
// - max_tokens: Legacy field, used by most models
// - max_completion_tokens: New field for o1/o3/gpt-5 families
//
// This transform applies the same rules to both fields:
// 1. If zero/omitted and default is set, fill with default
// 2. Cap at maxAllowed (model's maximum)
func (t *MaxTokensTransform) applyOpenAIChat(req *openai.ChatCompletionNewParams) {
	// Handle max_tokens (legacy field, used by most models)
	if req.MaxTokens.Valid() {
		// Fill default when client sends 0
		if req.MaxTokens.Value == 0 && t.DefaultMaxTokens > 0 {
			req.MaxTokens = param.NewOpt(t.DefaultMaxTokens)
		}
		// Cap at maxAllowed
		if req.MaxTokens.Value > t.MaxAllowed {
			req.MaxTokens = param.NewOpt(t.MaxAllowed)
		}
	} else {
		// Field omitted entirely - set default if available
		if t.DefaultMaxTokens > 0 {
			req.MaxTokens = param.NewOpt(t.DefaultMaxTokens)
		}
	}

	// Handle max_completion_tokens (new field for o1/o3/gpt-5)
	if req.MaxCompletionTokens.Valid() {
		// Fill default when client sends 0
		if req.MaxCompletionTokens.Value == 0 && t.DefaultMaxTokens > 0 {
			req.MaxCompletionTokens = param.NewOpt(t.DefaultMaxTokens)
		}
		// Cap at maxAllowed
		if req.MaxCompletionTokens.Value > t.MaxAllowed {
			req.MaxCompletionTokens = param.NewOpt(t.MaxAllowed)
		}
	}
	// Note: Don't auto-fill max_completion_tokens if omitted - it's an explicit opt-in
	// for o1/o3/gpt-5 models. Only cap if explicitly set.
}

// applyOpenAIResponses handles max_output_tokens for OpenAI Responses API.
//
// OpenAI Responses API uses max_output_tokens to control output length.
// This transform applies the same rules as other protocols:
// 1. If zero/omitted and default is set, fill with default
// 2. Cap at maxAllowed (model's maximum)
func (t *MaxTokensTransform) applyOpenAIResponses(req *responses.ResponseNewParams) {
	if req.MaxOutputTokens.Valid() {
		// Fill default when client sends 0
		if req.MaxOutputTokens.Value == 0 && t.DefaultMaxTokens > 0 {
			req.MaxOutputTokens = param.NewOpt(t.DefaultMaxTokens)
		}
		// Cap at maxAllowed
		if req.MaxOutputTokens.Value > t.MaxAllowed {
			req.MaxOutputTokens = param.NewOpt(t.MaxAllowed)
		}
	} else {
		// Field omitted entirely - set default if available
		if t.DefaultMaxTokens > 0 {
			req.MaxOutputTokens = param.NewOpt(t.DefaultMaxTokens)
		}
	}
}
