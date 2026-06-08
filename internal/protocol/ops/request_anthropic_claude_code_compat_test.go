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
		{
			name: "system after user merges into the user turn",
			in: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
				systemMessage("be terse"),
			},
			// No consecutive user messages — the system content is folded in.
			want: []rt{{Role: "user", Text: "hello|be terse"}},
		},
		{
			name: "system after assistant is re-roled to user (alternation-safe)",
			in: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("hi")),
				anthropic.NewAssistantMessage(anthropic.NewTextBlock("hey")),
				systemMessage("reminder"),
			},
			want: []rt{
				{Role: "user", Text: "hi"},
				{Role: "assistant", Text: "hey"},
				{Role: "user", Text: "reminder"},
			},
		},
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
			name: "system following assistant then another system stays alternating",
			in: []anthropic.MessageParam{
				anthropic.NewAssistantMessage(anthropic.NewTextBlock("a")),
				systemMessage("s1"),
				systemMessage("s2"),
			},
			// s1 re-roled to user; s2 merges into that user — no consecutive users.
			want: []rt{
				{Role: "assistant", Text: "a"},
				{Role: "user", Text: "s1|s2"},
			},
		},
		{
			name: "leading system (defensive — beta forbids it) merges forward into next user",
			in: []anthropic.MessageParam{
				systemMessage("lead"),
				anthropic.NewUserMessage(anthropic.NewTextBlock("body")),
			},
			want: []rt{{Role: "user", Text: "lead|body"}},
		},
		{
			name: "leading system before assistant is flushed as a lone user turn",
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
	ApplyClaudeCodeCompatRoleRewrite(nil)             // must not panic
	ApplyClaudeCodeBetaCompatRoleRewrite(nil)         // must not panic
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
