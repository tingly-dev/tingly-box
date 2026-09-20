package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookupClaudeThinkingCaps(t *testing.T) {
	t.Run("exact dated id", func(t *testing.T) {
		caps, ok := LookupClaudeThinkingCaps("claude-opus-4-5-20251101")
		require.True(t, ok)
		assert.True(t, caps.ThinkingEnabled)
		assert.False(t, caps.ThinkingAdaptive)
		assert.True(t, caps.EffortLevels["high"])
		assert.False(t, caps.EffortLevels["max"])
	})

	t.Run("undated family name", func(t *testing.T) {
		caps, ok := LookupClaudeThinkingCaps("claude-sonnet-4-5")
		require.True(t, ok)
		assert.True(t, caps.ThinkingEnabled)
		assert.False(t, caps.ThinkingAdaptive)
		assert.Empty(t, caps.EffortLevels)
	})

	t.Run("cloud-provider decorations", func(t *testing.T) {
		caps, ok := LookupClaudeThinkingCaps("us.anthropic.claude-sonnet-4-5-20250929-v1:0")
		require.True(t, ok)
		assert.True(t, caps.ThinkingEnabled)
	})

	t.Run("most specific key wins over family prefix", func(t *testing.T) {
		// "claude-sonnet-4-6" must not resolve to the "claude-sonnet-4" family.
		caps, ok := LookupClaudeThinkingCaps("claude-sonnet-4-6")
		require.True(t, ok)
		assert.True(t, caps.ThinkingAdaptive)
		assert.True(t, caps.EffortLevels["max"])

		caps, ok = LookupClaudeThinkingCaps("claude-sonnet-4-20250514")
		require.True(t, ok)
		assert.False(t, caps.ThinkingAdaptive)
		assert.Empty(t, caps.EffortLevels)
	})

	t.Run("adaptive-only opus 4.7", func(t *testing.T) {
		caps, ok := LookupClaudeThinkingCaps("claude-opus-4-7")
		require.True(t, ok)
		assert.False(t, caps.ThinkingEnabled)
		assert.True(t, caps.ThinkingAdaptive)
		assert.True(t, caps.EffortLevels["xhigh"])
	})

	t.Run("no thinking at all", func(t *testing.T) {
		caps, ok := LookupClaudeThinkingCaps("claude-3-haiku-20240307")
		require.True(t, ok)
		assert.False(t, caps.ThinkingEnabled)
		assert.False(t, caps.ThinkingAdaptive)
		assert.Empty(t, caps.EffortLevels)
	})

	t.Run("pre-thinking generations are cataloged as thinking-free", func(t *testing.T) {
		for _, model := range []string{"claude-3-5-sonnet-20241022", "claude-3-opus-20240229"} {
			caps, ok := LookupClaudeThinkingCaps(model)
			require.True(t, ok, model)
			assert.False(t, caps.ThinkingEnabled)
			assert.False(t, caps.ThinkingAdaptive)
		}
		// Sonnet 3.7 introduced budget-based extended thinking.
		caps, ok := LookupClaudeThinkingCaps("claude-3-7-sonnet-20250219")
		require.True(t, ok)
		assert.True(t, caps.ThinkingEnabled)
		assert.False(t, caps.ThinkingAdaptive)
	})

	t.Run("latest generation is adaptive-only with effort", func(t *testing.T) {
		for _, model := range []string{"claude-opus-4-8", "claude-sonnet-5", "claude-opus-5", "claude-fable-5"} {
			caps, ok := LookupClaudeThinkingCaps(model)
			require.True(t, ok, model)
			assert.False(t, caps.ThinkingEnabled, model)
			assert.True(t, caps.ThinkingAdaptive, model)
			assert.True(t, caps.EffortLevels["xhigh"], model)
			assert.True(t, caps.EffortLevels["max"], model)
		}
	})

	t.Run("unknown models miss", func(t *testing.T) {
		_, ok := LookupClaudeThinkingCaps("gpt-5.2")
		assert.False(t, ok)
		_, ok = LookupClaudeThinkingCaps("claude-9-experimental")
		assert.False(t, ok)
		_, ok = LookupClaudeThinkingCaps("")
		assert.False(t, ok)
	})
}

func TestHasClaudeCatalogEntryUsesStrictNormalizedIdentity(t *testing.T) {
	for _, model := range []string{
		"claude-opus-4-7",
		"anthropic.claude-haiku-4-5-20251001-v1:0",
		"claude-haiku-4-5@20251001",
		"claude-opus-4-6-thinking",
	} {
		assert.True(t, hasClaudeCatalogEntry(model), model)
	}

	// Runtime lookup intentionally accepts decorated substrings, but that
	// permissiveness must not make the completeness invariant accept a new
	// model by borrowing an older family's capabilities.
	_, runtimeMatch := LookupClaudeThinkingCaps("claude-sonnet-4-7")
	assert.True(t, runtimeMatch, "regression fixture must demonstrate the permissive runtime lookup")
	assert.False(t, hasClaudeCatalogEntry("claude-sonnet-4-7"))
	assert.False(t, hasClaudeCatalogEntry("claude-opus-4-7-experimental"))
}
