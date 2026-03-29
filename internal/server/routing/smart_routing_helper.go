package routing

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"

	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
)

// ExtractRequestContext extracts RequestContext from different request types
func ExtractRequestContext(req interface{}) (*smartrouting.RequestContext, error) {
	switch r := req.(type) {
	case *openai.ChatCompletionNewParams:
		return smartrouting.ExtractContextFromOpenAIRequest(r), nil
	case *anthropic.MessageNewParams:
		return smartrouting.ExtractContextFromAnthropicRequest(r), nil
	case *anthropic.BetaMessageNewParams:
		return smartrouting.ExtractContextFromBetaRequest(r), nil
	default:
		return nil, nil
	}
}
