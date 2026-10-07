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

	status, body := env.SendDecision(t, `{"model":"`+model+`","state":"my invoice is wrong","questions":{
		"department":{"type":"choice","instructions":"Which team?","criteria":{"billing":"Payments","bug":"Bugs","sales":"Pricing"}},
		"is_urgent":{"type":"noul","instructions":"Urgent?"}}}`)
	require.Equal(t, 200, status, "body: %s", body)
	assert.Equal(t, "billing", gjson.GetBytes(body, "answers.department.choice").String())
	assert.Equal(t, 1.0, gjson.GetBytes(body, "answers.department.probabilities.billing").Float())
	assert.Equal(t, 1.0, gjson.GetBytes(body, "answers.is_urgent.noul").Float())
	assert.Greater(t, gjson.GetBytes(body, "usage.input_tokens").Int(), int64(0))
	// Gateway echoes the caller's model, not the routed vmodel id.
	assert.Equal(t, model, gjson.GetBytes(body, "model").String())

	// An unanswerable request is the caller's error and keeps its status.
	status, body = env.SendDecision(t, `{"model":"`+model+`","state":"x"}`)
	assert.Equal(t, 400, status, "body: %s", body)
}
