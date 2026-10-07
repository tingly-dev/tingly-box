package protocoltest_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	pt "github.com/tingly-dev/tingly-box/internal/protocoltest"
)

// Decisions through the real gateway to a real vmodel decisions service:
// routing by model, model rewrite upstream, model echo downstream, usage
// accounting, and client errors surfacing with their status.
func TestDecisions_GatewayToVModel(t *testing.T) {
	env := pt.NewTestEnv(t)
	defer env.Close()
	model := env.SetupDecisionRoute(t, "decision-first")

	status, body := env.SendDecision(t,
		`{"model":"`+model+`","options":["billing","bug","sales"],"context":"my invoice is wrong"}`)
	require.Equal(t, 200, status, "body: %s", body)
	assert.Equal(t, "billing", gjson.GetBytes(body, "answer").String())
	assert.Equal(t, 1.0, gjson.GetBytes(body, "probabilities.billing").Float())
	// Gateway echoes the caller's model, not the routed vmodel id.
	assert.Equal(t, model, gjson.GetBytes(body, "model").String())

	// Jev-style questions map passes through the same endpoint.
	status, body = env.SendDecision(t,
		`{"model":"`+model+`","questions":{"urgent":{"type":"choice","options":["yes","no"]}}}`)
	require.Equal(t, 200, status, "body: %s", body)
	assert.Equal(t, "yes", gjson.GetBytes(body, "answers.urgent").String())

	// An unanswerable request is the caller's error and keeps its status.
	status, body = env.SendDecision(t, `{"model":"`+model+`"}`)
	assert.Equal(t, 400, status, "body: %s", body)
}
