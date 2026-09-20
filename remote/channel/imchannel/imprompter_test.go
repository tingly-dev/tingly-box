package imchannel

import (
	"strings"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/remote/control/ask"
)

// Test_AskUserQuestionKeyboard_AcceptsHeterogeneousShapes verifies that the
// AskUserQuestion keyboard builder renders option buttons regardless of which
// concrete slice/map type the caller used for "questions" / "options". The
// production code path goes through `[]interface{}` and `map[string]interface{}`,
// but agentboot callers serializing from typed structs may emit
// `[]map[string]any` instead — IMPrompter must accept both rather than
// silently degrading to the default Approve/Deny keyboard.
func Test_AskUserQuestionKeyboard_AcceptsHeterogeneousShapes(t *testing.T) {
	cases := []struct {
		name  string
		input map[string]interface{}
	}{
		{
			name: "interface_slice_with_interface_options",
			input: map[string]interface{}{
				"questions": []interface{}{
					map[string]interface{}{
						"question": "color?",
						"options": []interface{}{
							map[string]interface{}{"label": "red"},
							map[string]interface{}{"label": "blue"},
						},
					},
				},
			},
		},
		{
			name: "typed_slice_of_maps",
			input: map[string]interface{}{
				"questions": []map[string]interface{}{
					{
						"question": "color?",
						"options": []map[string]interface{}{
							{"label": "red"},
							{"label": "blue"},
						},
					},
				},
			},
		},
		{
			name: "typed_slice_of_any",
			input: map[string]interface{}{
				"questions": []any{
					map[string]any{
						"question": "color?",
						"options": []any{
							map[string]any{"label": "red"},
							map[string]any{"label": "blue"},
						},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewIMPrompter(nil)
			req := ask.Request{
				ID:       "req-1",
				ToolName: "AskUserQuestion",
				Input:    tc.input,
			}

			kb := p.buildAskUserQuestionKeyboard(req)

			// Walk the rendered buttons and assert two perm:option payloads
			// exist, one per declared option.
			labels := []string{}
			callbacks := []string{}
			for _, row := range kb.InlineKeyboard {
				for _, b := range row {
					labels = append(labels, b.Text)
					callbacks = append(callbacks, b.Payload.FlatCallbackData())
				}
			}

			countOptionCallbacks := 0
			for _, cb := range callbacks {
				if strings.HasPrefix(cb, "perm:option:") {
					countOptionCallbacks++
				}
			}
			if countOptionCallbacks != 2 {
				t.Fatalf("expected 2 perm:option callbacks, got %d. labels=%v callbacks=%v",
					countOptionCallbacks, labels, callbacks)
			}

			// Sanity: the labels must include the option names ("red", "blue").
			joined := strings.Join(labels, "|")
			if !strings.Contains(joined, "red") || !strings.Contains(joined, "blue") {
				t.Fatalf("expected option labels red and blue in keyboard, got %v", labels)
			}
		})
	}
}

// Test_NormalizeQuestionList_AcceptsHeterogeneousShapes verifies the helper
// the fix introduces at imprompter.go to normalize the questions slice into
// `[]map[string]any` regardless of caller-side concrete types.
func Test_NormalizeQuestionList_AcceptsHeterogeneousShapes(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want int
	}{
		{"interface_slice", []interface{}{map[string]any{}, map[string]any{}}, 2},
		{"map_slice", []map[string]any{{}, {}}, 2},
		{"any_slice", []any{map[string]any{}}, 1},
		{"nil", nil, 0},
		{"wrong_type", "not a slice", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := len(normalizeQuestionList(tc.in))
			if got != tc.want {
				t.Fatalf("normalizeQuestionList(%v) = len %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// Test_GetPendingRequestsForChat_OrdersByCreatedAtDescending guards the fix
// for a real bug: pendingRequests is a map, so without an explicit sort the
// order GetPendingRequestsForChat returns is whatever Go's unspecified map
// iteration happens to produce. bot.HandlePromptTextReply treats index 0 as
// "the latest pending request" — before this fix, that pick was effectively
// random whenever a chat had more than one pending request; a text reply
// could resolve against the wrong one. See .design/imbot-output.md §8.
func Test_GetPendingRequestsForChat_OrdersByCreatedAtDescending(t *testing.T) {
	p := NewIMPrompter(nil)

	base := time.Now()
	// Insertion order deliberately does not match chronological order, and a
	// request from a different chat is mixed in to confirm it's filtered out.
	p.pendingRequests = map[string]*pendingIMRequest{
		"req-mid": {
			request:   ask.Request{ID: "req-mid"},
			chatID:    "chat-1",
			createdAt: base.Add(1 * time.Second),
		},
		"req-other-chat": {
			request:   ask.Request{ID: "req-other-chat"},
			chatID:    "chat-2",
			createdAt: base.Add(3 * time.Second),
		},
		"req-new": {
			request:   ask.Request{ID: "req-new"},
			chatID:    "chat-1",
			createdAt: base.Add(2 * time.Second),
		},
		"req-old": {
			request:   ask.Request{ID: "req-old"},
			chatID:    "chat-1",
			createdAt: base,
		},
	}

	got := p.GetPendingRequestsForChat("chat-1")
	if len(got) != 3 {
		t.Fatalf("expected 3 requests for chat-1, got %d: %+v", len(got), got)
	}
	wantOrder := []string{"req-new", "req-mid", "req-old"}
	for i, id := range wantOrder {
		if got[i].ID != id {
			t.Errorf("position %d: got %q, want %q (most-recent-first)", i, got[i].ID, id)
		}
	}
}

// Test_GetPendingRequestsForChat_PrefersRemoteAgentOverNotify guards the
// source tie-break: a remote_agent-sourced request must sort before a
// notify-sourced one even when the notify one was created more recently —
// the user typing in a chat with an active @cc/SmartGuide conversation is
// almost always continuing it, not answering an older background notify
// ping. See the Source type doc comment and .design/imbot-output.md §8 for
// why this is a heuristic, not a guarantee.
func Test_GetPendingRequestsForChat_PrefersRemoteAgentOverNotify(t *testing.T) {
	p := NewIMPrompter(nil)

	base := time.Now()
	p.pendingRequests = map[string]*pendingIMRequest{
		"req-notify-newer": {
			request:   ask.Request{ID: "req-notify-newer", Source: ask.SourceNotify},
			chatID:    "chat-1",
			createdAt: base.Add(1 * time.Second), // created after the agent request
		},
		"req-agent-older": {
			request:   ask.Request{ID: "req-agent-older", Source: ask.SourceRemoteAgent},
			chatID:    "chat-1",
			createdAt: base,
		},
	}

	got := p.GetPendingRequestsForChat("chat-1")
	if len(got) != 2 {
		t.Fatalf("expected 2 requests for chat-1, got %d: %+v", len(got), got)
	}
	if got[0].ID != "req-agent-older" {
		t.Errorf("position 0: got %q, want %q — remote_agent must outrank a more recent notify request", got[0].ID, "req-agent-older")
	}
	if got[1].ID != "req-notify-newer" {
		t.Errorf("position 1: got %q, want %q", got[1].ID, "req-notify-newer")
	}
}
