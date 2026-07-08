package server

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// chainNames runs the (zero-config) chain builder and returns the ordered
// transform names. A bare ProtocolHandler has nil deps, so recording and MCP are off —
// leaving the canonical base order plus whatever rule transforms are slotted in.
func chainNames(t *testing.T, preBase, preVendor []transform.Transform) []string {
	t.Helper()
	h := &ProtocolHandler{}
	chain, err := h.buildTransformChain(nil, protocol.TypeOpenAIChat, typ.ScenarioGlobal, nil, preBase, preVendor)
	require.NoError(t, err)

	var names []string
	for _, tr := range chain.GetTransforms() {
		names = append(names, tr.Name())
	}
	return names
}

// TestBuildTransformChain_BaseOrder pins the canonical base order with no rule
// transforms: protocol convert → normalize → vendor finalize.
func TestBuildTransformChain_BaseOrder(t *testing.T) {
	names := chainNames(t, nil, nil)
	assert.Equal(t, []string{"base_convert", "consistency_normalize", "vendor_adjust"}, names)
}

// TestBuildTransformChain_PreVendorBeforeVendor is the regression guard for the
// core fix: rule preVendor transforms (target-shape transforms) must run after
// Consistency and BEFORE Vendor, so Vendor remains the final, immutable step
// facing the provider.
func TestBuildTransformChain_PreVendorBeforeVendor(t *testing.T) {
	preVendor := []transform.Transform{
		transform.NewRuleThinkingTransform(typ.ThinkingEffortHigh),
		transform.NewOpenAIMaxTokensRewriteTransform(true, false),
	}
	names := chainNames(t, nil, preVendor)

	consistency := slices.Index(names, "consistency_normalize")
	vendor := slices.Index(names, "vendor_adjust")
	thinking := slices.Index(names, "rule_thinking")
	maxTokens := slices.Index(names, "openai_max_tokens_rewrite")

	require.NotEqual(t, -1, consistency)
	require.NotEqual(t, -1, vendor)
	require.NotEqual(t, -1, thinking)
	require.NotEqual(t, -1, maxTokens)

	// preVendor transforms sit after Consistency...
	assert.Greater(t, thinking, consistency, "rule_thinking must run after consistency_normalize")
	assert.Greater(t, maxTokens, consistency, "openai_max_tokens_rewrite must run after consistency_normalize")
	// ...and strictly before Vendor — nothing mutates the request after Vendor.
	assert.Less(t, thinking, vendor, "rule_thinking must run before vendor_adjust")
	assert.Less(t, maxTokens, vendor, "openai_max_tokens_rewrite must run before vendor_adjust")
}

// TestBuildTransformChain_PreBaseBeforeBase verifies the inbound slot: pre-Base
// rule transforms run before protocol conversion so they see the client shape.
func TestBuildTransformChain_PreBaseBeforeBase(t *testing.T) {
	preBase := []transform.Transform{transform.NewOpenAICursorCompatTransform()}
	names := chainNames(t, preBase, nil)

	cursor := slices.Index(names, "openai_cursor_compat")
	base := slices.Index(names, "base_convert")

	require.NotEqual(t, -1, base)
	assert.Equal(t, 0, cursor, "pre-Base rule transform must be first in the chain")
	assert.Less(t, cursor, base, "pre-Base rule transform must run before base_convert")
}
