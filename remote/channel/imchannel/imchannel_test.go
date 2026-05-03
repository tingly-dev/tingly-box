package imchannel

import (
	"context"
	"testing"

	"github.com/tingly-dev/tingly-box/agentboot/ask"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/interaction"
)

type fakeSender struct {
	lastTarget string
	lastText   string
	err        error
}

func (s *fakeSender) SendMessage(ctx context.Context, target string, opts *imbot.SendMessageOptions) (*imbot.SendResult, error) {
	s.lastTarget = target
	if opts != nil {
		s.lastText = opts.Text
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
	c := New("bot-1", "telegram", s, nil)
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
}

func TestPromptTranslatesPermission(t *testing.T) {
	p := &fakePrompter{result: ask.Result{Approved: true, Reason: "ok"}}
	c := New("bot-1", "telegram", nil, p)
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
	c := New("bot-1", "telegram", nil, p)
	ix := interaction.Interaction{
		ID:   "ix2",
		Kind: interaction.KindChoose,
		Meta: map[string]any{
			"tool_name": "AskUserQuestion",
			"tool_input": map[string]interface{}{
				"questions": []interface{}{map[string]interface{}{"question": "q1", "options": []interface{}{"a", "b"}}},
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
	c := New("bot-1", "telegram", nil, p)
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
