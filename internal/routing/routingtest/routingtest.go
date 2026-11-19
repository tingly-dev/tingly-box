// Package routingtest holds the routing fixtures that tests outside package
// routing share (routing_test's pipeline benchmarks, internal/servertest).
// Package routing's own fixtures and mocks live in its _test.go files and are
// not importable; these two constructors only build plain values, so the
// package imports nothing from routing and adds no import cycle.
package routingtest

import (
	"github.com/openai/openai-go/v3"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

// Service returns a load-balancer service with weight 1.
func Service(provider, model string, active bool) *loadbalance.Service {
	return &loadbalance.Service{
		Provider: provider,
		Model:    model,
		Weight:   1,
		Active:   active,
	}
}

// OpenAIRequest returns a minimal OpenAI chat request for model.
func OpenAIRequest(model string) *openai.ChatCompletionNewParams {
	return &openai.ChatCompletionNewParams{
		Model: openai.ChatModel(model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("hello"),
		},
	}
}
