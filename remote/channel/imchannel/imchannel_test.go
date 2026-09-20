package imchannel

import (
	"context"
	"testing"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/control/ask"
	"github.com/tingly-dev/tingly-box/remote/interaction"
)

type fakeSender struct {
	lastTarget    string
	lastText      string
	lastParseMode imbot.ParseMode
	lastMetadata  map[string]any
	err           error
}

func (s *fakeSender) SendMessage(ctx context.Context, target string, opts *imbot.SendMessageOptions) (*imbot.SendResult, error) {
	s.lastTarget = target
	if opts != nil {
		s.lastText = opts.Text
		s.lastParseMode = opts.ParseMode
		s.lastMetadata = opts.Metadata
	}
	if s.err != nil {
		return nil, s.err
	}
	return &imbot.SendResult{MessageID: "m1"}, nil
}

type fakePrompter struct {
	gotReq ask.Request
	result ask.Result
	err    error
}

func (p *fakePrompter) Prompt(ctx context.Context, req ask.Request) (ask.Result, error) {
	p.gotReq = req
	if p.err != nil {
		return ask.Result{}, p.err
	}
	return p.result, nil
}

func TestSendComposesTitleAndBody(t *testing.T) {
	s := &fakeSender{}
	c := New("bot-1", "telegram", s, nil, nil)
	err := c.Send(context.Background(), channel.Target{ChatID: "chat-1"}, interaction.Notification{
		Title: "Claude",
		Body:  "task done",
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.lastTarget != "chat-1" {
		t.Fatalf("target = %q", s.lastTarget)
	}
	if s.lastText != "Claude\ntask done" {
		t.Fatalf("text = %q", s.lastText)
	}
	// Send parses the body as markdown so platform renderers exercise it; the
	// channel advertises Markdown support and the prompter path already does
	// the same.
	if s.lastParseMode != imbot.ParseModeMarkdown {
		t.Fatalf("ParseMode = %q, want %q", s.lastParseMode, imbot.ParseModeMarkdown)
	}
}

func TestSendInjectsContextToken(t *testing.T) {
	s := &fakeSender{}
	// tokenLookup returns a token only for userA.
	lookup := func(target string) string {
		if target == "userA" {
			return "tok-1"
		}
		return ""
	}
	c := New("bot-1", "weixin", s, nil, lookup)

	// Target with a persisted token -> metadata carries context_token.
	if err := c.Send(context.Background(), channel.Target{ChatID: "userA"}, interaction.Notification{Body: "hi"}); err != nil {
		t.Fatal(err)
	}
	if got := s.lastMetadata["context_token"]; got != "tok-1" {
		t.Fatalf("userA context_token = %v, want %q", got, "tok-1")
	}

	// Target with no token -> metadata is nil (no token injected), send still succeeds.
	if err := c.Send(context.Background(), channel.Target{ChatID: "userB"}, interaction.Notification{Body: "hi"}); err != nil {
		t.Fatal(err)
	}
	if s.lastMetadata != nil {
		t.Fatalf("userB metadata = %v, want nil", s.lastMetadata)
	}
}

func TestSendNoTokenLookupLeavesMetadataNil(t *testing.T) {
	s := &fakeSender{}
	c := New("bot-1", "telegram", s, nil, nil) // no tokenLookup
	if err := c.Send(context.Background(), channel.Target{ChatID: "chat-1"}, interaction.Notification{Body: "hi"}); err != nil {
		t.Fatal(err)
	}
	if s.lastMetadata != nil {
		t.Fatalf("metadata = %v, want nil when no tokenLookup", s.lastMetadata)
	}
}

func TestPromptTranslatesPermission(t *testing.T) {
	p := &fakePrompter{result: ask.Result{Approved: true, Reason: "ok"}}
	c := New("bot-1", "telegram", nil, p, nil)
	ix := interaction.Interaction{
		ID:    "ix1",
		Kind:  interaction.KindConfirm,
		Title: "Run command",
		Body:  "ls",
		Meta: map[string]any{
			"tool_name":  "Bash",
			"session_id": "s1",
		},
	}
	reply, err := c.Prompt(context.Background(), channel.Target{ChatID: "c1"}, ix)
	if err != nil {
		t.Fatal(err)
	}
	if p.gotReq.Type != ask.TypePermission {
		t.Fatalf("expected TypePermission, got %v", p.gotReq.Type)
	}
	if p.gotReq.ToolName != "Bash" {
		t.Fatalf("tool_name not propagated: %q", p.gotReq.ToolName)
	}
	if p.gotReq.SessionID != "s1" {
		t.Fatalf("session_id not propagated: %q", p.gotReq.SessionID)
	}
	if p.gotReq.Source != ask.SourceNotify {
		t.Fatalf("Prompt (Flow A, scenario-plugin channel) must tag requests ask.SourceNotify, got %q", p.gotReq.Source)
	}
	if reply.Status != interaction.StatusAnswered {
		t.Fatalf("status = %v", reply.Status)
	}
	if reply.Selected != "allow" {
		t.Fatalf("selected = %q", reply.Selected)
	}
}

func TestPromptTranslatesQuestion(t *testing.T) {
	p := &fakePrompter{result: ask.Result{
		Approved:     true,
		UpdatedInput: map[string]interface{}{"answers": map[string]interface{}{"q1": "a"}},
	}}
	c := New("bot-1", "telegram", nil, p, nil)
	ix := interaction.Interaction{
		ID:   "ix2",
		Kind: interaction.KindChoose,
		Meta: map[string]any{
			"tool_name": "AskUserQuestion",
			"tool_input": map[string]any{
				"questions": []any{map[string]any{"question": "q1", "options": []any{"a", "b"}}},
			},
		},
	}
	reply, err := c.Prompt(context.Background(), channel.Target{ChatID: "c1"}, ix)
	if err != nil {
		t.Fatal(err)
	}
	if p.gotReq.Type != ask.TypeQuestion {
		t.Fatalf("expected TypeQuestion, got %v", p.gotReq.Type)
	}
	if reply.Status != interaction.StatusAnswered {
		t.Fatalf("status = %v", reply.Status)
	}
	if reply.Meta["updated_input"] == nil {
		t.Fatalf("expected updated_input meta")
	}
}

func TestPromptCancelMappedToStatusCancelled(t *testing.T) {
	p := &fakePrompter{result: ask.Result{Approved: false, Reason: "cancel"}}
	c := New("bot-1", "telegram", nil, p, nil)
	reply, err := c.Prompt(context.Background(), channel.Target{ChatID: "c"}, interaction.Interaction{ID: "ix3", Kind: interaction.KindConfirm})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Status != interaction.StatusCancelled {
		t.Fatalf("expected cancelled, got %v", reply.Status)
	}
	if reply.Selected != "" {
		t.Fatalf("cancelled reply should not set Selected, got %q", reply.Selected)
	}
}
