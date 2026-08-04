package imchannel

import (
	"strings"
	"testing"

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
