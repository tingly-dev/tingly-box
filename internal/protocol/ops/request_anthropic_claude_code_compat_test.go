package ops

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// systemMessage builds a message with the non-standard "system" role carrying a
// single text block.
func systemMessage(text string) anthropic.MessageParam {
	return anthropic.MessageParam{
		Role:    "system",
		Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(text)},
	}
}

// roleTexts flattens a message slice into (role, joined-text) pairs for assertions.
func roleTexts(msgs []anthropic.MessageParam) []struct {
	Role string
	Text string
} {
	out := make([]struct {
		Role string
		Text string
	}, 0, len(msgs))
	for _, m := range msgs {
		var text string
		for _, b := range m.Content {
			if b.OfText != nil {
				if text != "" {
					text += "|"
				}
				text += b.OfText.Text
			}
		}
		out = append(out, struct {
			Role string
			Text string
		}{Role: string(m.Role), Text: text})
	}
	return out
}

func TestApplyClaudeCodeCompatRoleRewrite(t *testing.T) {
	type rt = struct {
		Role string
		Text string
	}

	tests := []struct {
		name string
		in   []anthropic.MessageParam
		want []rt
	}{
		// ── Exhaustive (prev, next) neighbour enumeration ──────────────────
		{
			// prev=user, next=user → collapse all three into one user turn.
			name: "case1: user, system, user collapses into one user",
			in: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("u1")),
				systemMessage("s"),
				anthropic.NewUserMessage(anthropic.NewTextBlock("u2")),
			},
			want: []rt{{Role: "user", Text: "u1|s|u2"}},
		},
		{
			// prev=user, next=assistant → merge backward into the preceding user.
			name: "case2: user, system, assistant merges backward",
			in: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("u")),
				systemMessage("s"),
				anthropic.NewAssistantMessage(anthropic.NewTextBlock("a")),
			},
			want: []rt{
				{Role: "user", Text: "u|s"},
				{Role: "assistant", Text: "a"},
			},
		},
		{
			// prev=user, next=∅ (trailing) → merge backward into the preceding user.
			name: "case3: user, system (trailing) merges backward",
			in: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("u")),
				systemMessage("s"),
			},
			want: []rt{{Role: "user", Text: "u|s"}},
		},
		{
			// prev=assistant, next=user → merge forward into the following user.
			// This is the regression: the old eager re-role produced two
			// consecutive user messages here.
			name: "case4: assistant, system, user merges forward",
			in: []anthropic.MessageParam{
				anthropic.NewAssistantMessage(anthropic.NewTextBlock("a")),
				systemMessage("s"),
				anthropic.NewUserMessage(anthropic.NewTextBlock("u")),
			},
			want: []rt{
				{Role: "assistant", Text: "a"},
				{Role: "user", Text: "s|u"},
			},
		},
		{
			// prev=assistant, next=assistant → stand alone as its own user turn.
			name: "case5: assistant, system, assistant stands alone",
			in: []anthropic.MessageParam{
				anthropic.NewAssistantMessage(anthropic.NewTextBlock("a1")),
				systemMessage("s"),
				anthropic.NewAssistantMessage(anthropic.NewTextBlock("a2")),
			},
			want: []rt{
				{Role: "assistant", Text: "a1"},
				{Role: "user", Text: "s"},
				{Role: "assistant", Text: "a2"},
			},
		},
		{
			// prev=assistant, next=∅ → stand alone as its own user turn.
			name: "case6: assistant, system (trailing) stands alone",
			in: []anthropic.MessageParam{
				anthropic.NewAssistantMessage(anthropic.NewTextBlock("a")),
				systemMessage("s"),
			},
			want: []rt{
				{Role: "assistant", Text: "a"},
				{Role: "user", Text: "s"},
			},
		},
		{
			// prev=∅, next=user → merge forward into the following user.
			name: "case7: leading system, user merges forward",
			in: []anthropic.MessageParam{
				systemMessage("lead"),
				anthropic.NewUserMessage(anthropic.NewTextBlock("body")),
			},
			want: []rt{{Role: "user", Text: "lead|body"}},
		},
		{
			// prev=∅, next=assistant → stand alone as its own user turn.
			name: "case8: leading system, assistant stands alone",
			in: []anthropic.MessageParam{
				systemMessage("lead"),
				anthropic.NewAssistantMessage(anthropic.NewTextBlock("a")),
			},
			want: []rt{
				{Role: "user", Text: "lead"},
				{Role: "assistant", Text: "a"},
			},
		},
		{
			// prev=∅, next=∅ → the sole message stands alone as a user turn.
			name: "case9: system is the only message",
			in: []anthropic.MessageParam{
				systemMessage("only"),
			},
			want: []rt{{Role: "user", Text: "only"}},
		},
		// ── Multi-system and longer-sequence stress cases ──────────────────
		{
			name: "consecutive systems after user all collapse into one user turn",
			in: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("q")),
				systemMessage("note1"),
				systemMessage("note2"),
			},
			want: []rt{{Role: "user", Text: "q|note1|note2"}},
		},
		{
			// Regression: two systems between assistant and user must all flow
			// forward into the user — the old code emitted user(s1|s2),user(body).
			name: "assistant, system, system, user all merge forward",
			in: []anthropic.MessageParam{
				anthropic.NewAssistantMessage(anthropic.NewTextBlock("a")),
				systemMessage("s1"),
				systemMessage("s2"),
				anthropic.NewUserMessage(anthropic.NewTextBlock("body")),
			},
			want: []rt{
				{Role: "assistant", Text: "a"},
				{Role: "user", Text: "s1|s2|body"},
			},
		},
		{
			name: "trailing consecutive systems after assistant collapse into one user",
			in: []anthropic.MessageParam{
				anthropic.NewAssistantMessage(anthropic.NewTextBlock("a")),
				systemMessage("s1"),
				systemMessage("s2"),
			},
			want: []rt{
				{Role: "assistant", Text: "a"},
				{Role: "user", Text: "s1|s2"},
			},
		},
		{
			// A realistic agentic turn: tool_result user, a reminder, then the
			// model's reply, then a fresh reminder before the next user input.
			name: "long interleaved conversation stays alternating",
			in: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("u1")),
				anthropic.NewAssistantMessage(anthropic.NewTextBlock("a1")),
				anthropic.NewUserMessage(anthropic.NewTextBlock("tool_result")),
				systemMessage("reminder1"),
				anthropic.NewAssistantMessage(anthropic.NewTextBlock("a2")),
				systemMessage("reminder2"),
				anthropic.NewUserMessage(anthropic.NewTextBlock("u2")),
			},
			want: []rt{
				{Role: "user", Text: "u1"},
				{Role: "assistant", Text: "a1"},
				{Role: "user", Text: "tool_result|reminder1"}, // backward into prev user
				{Role: "assistant", Text: "a2"},
				{Role: "user", Text: "reminder2|u2"}, // forward into next user
			},
		},
		{
			name: "no system roles is a pure pass-through",
			in: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("u")),
				anthropic.NewAssistantMessage(anthropic.NewTextBlock("a")),
			},
			want: []rt{
				{Role: "user", Text: "u"},
				{Role: "assistant", Text: "a"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &anthropic.MessageNewParams{Messages: tc.in}
			ApplyClaudeCodeCompatRoleRewrite(req)
			got := roleTexts(req.Messages)
			if len(got) != len(tc.want) {
				t.Fatalf("message count = %d, want %d: %+v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i].Role != tc.want[i].Role || got[i].Text != tc.want[i].Text {
					t.Errorf("message[%d] = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
			// Invariant: never two consecutive user messages after the rewrite.
			for i := 1; i < len(got); i++ {
				if got[i].Role == "user" && got[i-1].Role == "user" {
					t.Errorf("consecutive user messages at [%d],[%d]: %+v", i-1, i, got)
				}
			}
		})
	}
}

func TestApplyClaudeCodeCompatRoleRewrite_Nil(t *testing.T) {
	ApplyClaudeCodeCompatRoleRewrite(nil)     // must not panic
	ApplyClaudeCodeBetaCompatRoleRewrite(nil) // must not panic
}

func TestApplyClaudeCodeBetaCompatRoleRewrite(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hello")),
			{
				Role:    "system",
				Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("be terse")},
			},
		},
	}
	ApplyClaudeCodeBetaCompatRoleRewrite(req)

	if len(req.Messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(req.Messages))
	}
	if got := req.Messages[0].Role; got != anthropic.BetaMessageParamRoleUser {
		t.Errorf("role = %q, want user", got)
	}
	var text string
	for _, b := range req.Messages[0].Content {
		if b.OfText != nil {
			if text != "" {
				text += "|"
			}
			text += b.OfText.Text
		}
	}
	if text != "hello|be terse" {
		t.Errorf("merged text = %q, want %q", text, "hello|be terse")
	}
}
