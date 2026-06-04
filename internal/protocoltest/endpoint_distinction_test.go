//go:build e2e
// +build e2e

package protocoltest

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// TestEndpointDistinction_ChatVsResponses proves the harness routes to the
// correct OpenAI endpoint. chat and responses are two distinct protocols, so a
// target=openai_responses route must hit /v1/responses, and a target=openai_chat
// route must hit /v1/chat/completions — never the other one.
//
// Before OpenAIEndpointMode was wired into SetupRoute, ResolveOpenAIEndpoint
// defaulted every OpenAI provider to chat, so target=openai_responses silently
// fell back to /chat/completions. This test guards against that regression.
func TestEndpointDistinction_ChatVsResponses(t *testing.T) {
	t.Run("target=openai_responses hits /responses", func(t *testing.T) {
		env := NewTestEnv(t)
		defer env.Close()

		s := TextScenario()
		env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeOpenAIResponses, s)
		res := env.SendAs(t, protocol.TypeAnthropicV1, protocol.TypeOpenAIResponses, s, false)

		assert.Equal(t, 200, res.HTTPStatus)
		assert.Equal(t, 1, env.virtual.EndpointHits(EndpointResponses),
			"target=openai_responses must forward to /v1/responses")
		assert.Equal(t, 0, env.virtual.EndpointHits(EndpointChat),
			"target=openai_responses must NOT forward to /v1/chat/completions")
	})

	t.Run("target=openai_chat hits /chat/completions", func(t *testing.T) {
		env := NewTestEnv(t)
		defer env.Close()

		s := TextScenario()
		env.SetupRoute(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, s)
		res := env.SendAs(t, protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, s, false)

		assert.Equal(t, 200, res.HTTPStatus)
		assert.Equal(t, 1, env.virtual.EndpointHits(EndpointChat),
			"target=openai_chat must forward to /v1/chat/completions")
		assert.Equal(t, 0, env.virtual.EndpointHits(EndpointResponses),
			"target=openai_chat must NOT forward to /v1/responses")
	})
}
