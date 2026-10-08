package promptcache

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// anthropicTurn builds turn n of a growing Anthropic conversation: n user
// questions with the n-1 answers between them, and Claude Code's rolling
// breakpoint on the last question (when breakpoint). Turn n+1 appends one
// answer and one question to turn n.
func anthropicTurn(n int, breakpoint bool, system string) []byte {
	ephemeral := map[string]any{"type": "ephemeral"}
	var msgs []map[string]any
	for i := 0; i < n; i++ {
		if i > 0 {
			msgs = append(msgs, map[string]any{"role": "assistant", "content": []map[string]any{{"type": "text", "text": fmt.Sprintf("answer %d with some padding text", i-1)}}})
		}
		msgs = append(msgs, map[string]any{"role": "user", "content": []map[string]any{{"type": "text", "text": fmt.Sprintf("question %d with some padding text", i)}}})
	}
	if breakpoint {
		last := msgs[len(msgs)-1]["content"].([]map[string]any)
		last[0]["cache_control"] = ephemeral
	}
	body, _ := json.Marshal(map[string]any{
		"system":   []map[string]any{{"type": "text", "text": system, "cache_control": ephemeral}},
		"tools":    []map[string]any{{"name": "read", "input_schema": map[string]any{"type": "object"}}},
		"messages": msgs,
	})
	return body
}

func observe(t *testing.T, s *Simulator, d Discipline, raw []byte) Result {
	t.Helper()
	blocks, err := AnthropicBlocks(raw)
	require.NoError(t, err)
	return s.Observe("anthropic/m", d, blocks)
}

func TestExplicitGrowingConversationHits(t *testing.T) {
	s := New()
	first := observe(t, s, Explicit, anthropicTurn(1, true, "system prompt"))
	assert.Zero(t, first.CachedTokens)
	assert.Equal(t, first.PromptTokens, first.WriteTokens, "first turn writes up to its last breakpoint")
	assert.Equal(t, "tools[0]", first.FirstUncached)

	second := observe(t, s, Explicit, anthropicTurn(2, true, "system prompt"))
	assert.Equal(t, first.PromptTokens, second.CachedTokens, "turn N+1 reads all of turn N")
	assert.Equal(t, "messages[1].content[0]", second.FirstUncached)
	assert.Equal(t, second.PromptTokens, second.CachedTokens+second.WriteTokens)
	assert.Zero(t, second.UncachedInputTokens())

	// The breakpoint rolled forward; reading turn N+1 again hits all of it.
	again := observe(t, s, Explicit, anthropicTurn(2, true, "system prompt"))
	assert.Equal(t, again.PromptTokens, again.CachedTokens)
	assert.Empty(t, again.FirstUncached)
}

func TestExplicitWithoutBreakpointsNeverCaches(t *testing.T) {
	s := New()
	noBP := func(n int) []byte {
		body, _ := json.Marshal(map[string]any{"messages": []map[string]any{
			{"role": "user", "content": fmt.Sprintf("hello %d", n)},
		}})
		return body
	}
	observe(t, s, Explicit, noBP(1))
	r := observe(t, s, Explicit, noBP(1))
	assert.Zero(t, r.CachedTokens, "Anthropic caches nothing without a breakpoint")
	assert.Zero(t, r.WriteTokens)
}

func TestAutomaticCachesWithoutBreakpoints(t *testing.T) {
	s := New()
	chat := func(n int) []byte {
		var msgs []map[string]any
		for i := 0; i < n; i++ {
			msgs = append(msgs, map[string]any{"role": "user", "content": fmt.Sprintf("message %d", i)})
		}
		body, _ := json.Marshal(map[string]any{"messages": msgs})
		return body
	}
	b1, _ := ChatBlocks(chat(3))
	s.Observe("chat/m", Automatic, b1)
	b2, _ := ChatBlocks(chat(4))
	r := s.Observe("chat/m", Automatic, b2)
	assert.Equal(t, 3, r.CachedBlocks)
	assert.Equal(t, "messages[3]", r.FirstUncached)
	assert.Zero(t, r.WriteTokens, "automatic caches report no writes")
}

func TestMovingBreakpointKeepsKey(t *testing.T) {
	withBP, _ := AnthropicBlocks(anthropicTurn(2, true, "s"))
	without, _ := AnthropicBlocks(anthropicTurn(2, false, "s"))
	require.Equal(t, len(withBP), len(without))
	for i := range withBP {
		assert.Equal(t, string(withBP[i].key), string(without[i].key), "block %s", withBP[i].Label)
	}
}

func TestChangedSystemMissesEverythingAfterTools(t *testing.T) {
	s := New()
	observe(t, s, Explicit, anthropicTurn(2, true, "system v1"))
	r := observe(t, s, Explicit, anthropicTurn(2, true, "system v2"))
	assert.Zero(t, r.CachedTokens, "tools carry no breakpoint, so nothing before the changed system block was written")
	assert.Equal(t, "tools[0]", r.FirstUncached)
}

func TestScopesAndTTL(t *testing.T) {
	s := New()
	now := time.Unix(0, 0)
	s.now = func() time.Time { return now }
	b, _ := ChatBlocks([]byte(`{"messages":[{"role":"user","content":"hi"}]}`))
	s.Observe("a", Automatic, b)
	assert.Zero(t, s.Observe("b", Automatic, b).CachedBlocks, "another scope is another cache")
	assert.Equal(t, 1, s.Observe("a", Automatic, b).CachedBlocks)
	now = now.Add(DefaultTTL + time.Second)
	assert.Zero(t, s.Observe("a", Automatic, b).CachedBlocks, "expired")
}

func TestResponsesBlocks(t *testing.T) {
	b, err := ResponsesBlocks([]byte(`{"instructions":"be brief","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi","prompt_cache_breakpoint":{"type":"ephemeral"}}]}],"tools":[{"type":"function","name":"f"}]}`))
	require.NoError(t, err)
	require.Len(t, b, 3)
	assert.Equal(t, []string{"tools[0]", "instructions", "input[0]"}, []string{b[0].Label, b[1].Label, b[2].Label})
	assert.NotContains(t, string(b[2].key), "prompt_cache_breakpoint")
}

func TestReport(t *testing.T) {
	r := Result{PromptTokens: 1000, CachedTokens: 900, WriteTokens: 50, Blocks: 10, CachedBlocks: 9, FirstUncached: "messages[4]"}
	assert.Equal(t, "Prompt cache simulation: 900 of 1000 prompt tokens read from cache (90.0%), 50 written. Cached prefix: 9 of 10 blocks; first uncached block: messages[4].", r.Report())
	assert.Equal(t, int64(50), r.UncachedInputTokens())
}
