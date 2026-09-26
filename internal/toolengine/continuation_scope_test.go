package toolengine

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func decode[T any](t *testing.T, raw string) T {
	t.Helper()
	var out T
	require.NoError(t, json.Unmarshal([]byte(raw), &out))
	return out
}

// The store is process-wide: without a session a stored round would be
// shared by every sessionless request, so nothing is stored or resumed.
func TestContinuationKeyRequiresSession(t *testing.T) {
	require.Empty(t, continuationKey(typ.SessionID{}, "provider", "anthropic-beta"))
	require.NotEmpty(t, continuationKey(typ.SessionID{Source: "header", Value: "s"}, "provider", "anthropic-beta"))

	segment := []openai.ChatCompletionMessageParamUnion{openai.AssistantMessage("x")}
	StoreOpenAIContinuationSegment(typ.SessionID{}, "provider-"+t.Name(), segment)
	_, ok := PopOpenAIContinuationSegment(typ.SessionID{}, "provider-"+t.Name(), &openai.ChatCompletionNewParams{})
	require.False(t, ok)
}

func TestContinuationResumesOnlyItsFollowUp(t *testing.T) {
	const turn = `{"role":"assistant","content":[
		{"type":"tool_use","id":"toolu_owned","name":"tingly_box_mcp__builtin__echo","input":{}},
		{"type":"tool_use","id":"toolu_client","name":"get_weather","input":{}}]}`
	request := func(toolUseID string) string {
		return `{"model":"m","max_tokens":8,"messages":[
			{"role":"user","content":"go"},
			{"role":"assistant","content":[{"type":"tool_use","id":"` + toolUseID + `","name":"get_weather","input":{}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"` + toolUseID + `","content":"sunny"}]}]}`
	}
	chatRequest := func(toolCallID string) string {
		return `{"model":"m","messages":[
			{"role":"user","content":"go"},
			{"role":"assistant","tool_calls":[{"id":"` + toolCallID + `","type":"function","function":{"name":"get_weather","arguments":"{}"}}]},
			{"role":"tool","tool_call_id":"` + toolCallID + `","content":"sunny"}]}`
	}

	cases := []struct {
		name               string
		segment            any
		followUp, unrelate any
	}{
		{
			name:     "anthropic-v1",
			segment:  []anthropic.MessageParam{decode[anthropic.MessageParam](t, turn)},
			followUp: ptr(decode[anthropic.MessageNewParams](t, request("toolu_client"))),
			unrelate: ptr(decode[anthropic.MessageNewParams](t, request("toolu_other"))),
		},
		{
			name:     "anthropic-beta",
			segment:  []anthropic.BetaMessageParam{decode[anthropic.BetaMessageParam](t, turn)},
			followUp: ptr(decode[anthropic.BetaMessageNewParams](t, request("toolu_client"))),
			unrelate: ptr(decode[anthropic.BetaMessageNewParams](t, request("toolu_other"))),
		},
		{
			name: "openai-chat",
			segment: []openai.ChatCompletionMessageParamUnion{decode[openai.ChatCompletionMessageParamUnion](t, `{"role":"assistant","content":"","tool_calls":[
				{"id":"call_owned","type":"function","function":{"name":"tingly_box_mcp__builtin__echo","arguments":"{}"}},
				{"id":"call_client","type":"function","function":{"name":"get_weather","arguments":"{}"}}]}`)},
			followUp: ptr(decode[openai.ChatCompletionNewParams](t, chatRequest("call_client"))),
			unrelate: ptr(decode[openai.ChatCompletionNewParams](t, chatRequest("call_other"))),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newContinuationStore()
			key := continuationKey(typ.SessionID{Source: "header", Value: t.Name()}, "provider", tc.name)
			store.put(key, tc.segment)

			_, ok := store.popAnswered(key, tc.unrelate)
			require.False(t, ok, "an unrelated request does not consume the stored round")

			segment, ok := store.popAnswered(key, tc.followUp)
			require.True(t, ok, "the follow-up resumes it")
			require.Equal(t, tc.segment, segment)

			_, ok = store.popAnswered(key, tc.followUp)
			require.False(t, ok, "only once")
		})
	}
}

func ptr[T any](v T) *T { return &v }
