package probe

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func playgroundBase() *E2ERequest {
	return &E2ERequest{TargetType: E2ETargetRule, Scenario: "claude_code", RuleUUID: "r1"}
}

func TestValidateE2ERequest_Playground(t *testing.T) {
	t.Run("flags ok through TB", func(t *testing.T) {
		req := playgroundBase()
		req.Flags = typ.FlagOverlay{"skip_usage": json.RawMessage("false")}
		assert.NoError(t, ValidateE2ERequest(req))
	})
	t.Run("flags rejected on direct", func(t *testing.T) {
		req := &E2ERequest{TargetType: E2ETargetProvider, ProviderUUID: "p", Model: "m", Direct: true,
			Flags: typ.FlagOverlay{"skip_usage": json.RawMessage("true")}}
		assert.Equal(t, "flags", validationField(t, ValidateE2ERequest(req)))
	})
	t.Run("flags validated against registry", func(t *testing.T) {
		req := playgroundBase()
		req.Flags = typ.FlagOverlay{"bogus": json.RawMessage("true")}
		assert.Equal(t, "flags", validationField(t, ValidateE2ERequest(req)))
	})
	t.Run("body overrides must be JSON", func(t *testing.T) {
		req := playgroundBase()
		req.BodyOverrides = map[string]json.RawMessage{"temperature": json.RawMessage("0.5")}
		assert.NoError(t, ValidateE2ERequest(req))
		req.BodyOverrides = map[string]json.RawMessage{"temperature": json.RawMessage("{oops")}
		assert.Equal(t, "body_overrides", validationField(t, ValidateE2ERequest(req)))
	})
	t.Run("header names", func(t *testing.T) {
		req := playgroundBase()
		req.Headers = map[string]string{"X-Ok": "1"}
		assert.NoError(t, ValidateE2ERequest(req))
		req.Headers = map[string]string{"bad name": "1"}
		assert.Equal(t, "headers", validationField(t, ValidateE2ERequest(req)))
	})
	t.Run("routing", func(t *testing.T) {
		req := playgroundBase()
		req.Routing = RoutingPinned
		assert.NoError(t, ValidateE2ERequest(req))
		req.Routing = ProbeRouting("sideways")
		assert.Equal(t, "routing", validationField(t, ValidateE2ERequest(req)))
		prov := &E2ERequest{TargetType: E2ETargetProvider, ProviderUUID: "p", Model: "m", Routing: RoutingPinned}
		assert.Equal(t, "routing", validationField(t, ValidateE2ERequest(prov)))
	})
}

func TestCustomized(t *testing.T) {
	req := playgroundBase()
	assert.False(t, req.Customized())
	req.Headers = map[string]string{"X": "1"}
	assert.True(t, req.Customized())
	assert.True(t, (&E2ERequest{Client: ClientClaudeCode}).Customized())
}

func TestProbeRewrite(t *testing.T) {
	assert.Nil(t, playgroundBase().probeRewrite())
	req := playgroundBase()
	req.BodyOverrides = map[string]json.RawMessage{"temperature": json.RawMessage("1")}
	rw := req.probeRewrite()
	require.NotNil(t, rw)
	assert.JSONEq(t, "1", string(rw.Body["temperature"]))
}

func TestApplyCurlHeaderOverrides(t *testing.T) {
	h := map[string]string{"Content-Type": "application/json", "Authorization": "Bearer $TB_API_KEY"}
	applyCurlHeaderOverrides(h, map[string]string{"authorization": "", "X-Extra": "1"})
	assert.Equal(t, map[string]string{"Content-Type": "application/json", "X-Extra": "1"}, h)
}
