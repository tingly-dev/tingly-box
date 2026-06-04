//go:build e2e
// +build e2e

package protocoltest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// TestChainHop_RequiresTailRoute proves the round-trip actually chains two
// gateway passes: a chain-head route forwards back into the gateway carrying
// the tail model, so without the tail route the second hop has nowhere to land
// and the request fails. Adding the tail route makes it succeed. This guards
// against a "false pass" where the harness might silently short-circuit to a
// single conversion.
func TestChainHop_RequiresTailRoute(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Close()

	s := TextScenario()
	headModel := "idem-mech-head"
	// The model the head forwards downstream; matches the tail route once created.
	tailModel := "pv-openai_chat-to-anthropic_beta-text"

	// Head route only: A(anthropic_v1) → B(openai_chat), re-entering the gateway.
	env.setupChainHopRoute(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, s, headModel, tailModel)

	// Without the tail route, the second hop (model=tailModel) has no rule.
	res, err := env.sendModel(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, s.Name, headModel, false)
	require.NoError(t, err)
	assert.NotEqual(t, 200, res.HTTPStatus,
		"without tail route the round-trip must fail, got 200 — chaining not happening")

	// Add the tail route: B(openai_chat) → A'(anthropic_beta) → virtual server.
	env.SetupRoute(protocol.TypeOpenAIChat, protocol.TypeAnthropicBeta, s)
	require.Equal(t, tailModel, env.findRouteModel(protocol.TypeOpenAIChat, protocol.TypeAnthropicBeta, s.Name),
		"tail route model must match the model the head forwards")

	// Now the full chain resolves and returns the scenario content.
	res2, err := env.sendModel(protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, s.Name, headModel, false)
	require.NoError(t, err)
	assert.Equal(t, 200, res2.HTTPStatus, "with tail route the round-trip must succeed")
	assert.Contains(t, res2.Content, "Paris", "chained response must carry scenario content")
}
