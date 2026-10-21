package imchannel

import (
	"context"
	"errors"
	"testing"

	"github.com/tingly-dev/tingly-box/imbot"
)

// actionSender records the actions it was sent and can restate.
type actionSender struct {
	fakeSender
	actions    *imbot.ActionSet
	restated   imbot.RestateOptions
	restateErr error
}

func (s *actionSender) SendMessage(ctx context.Context, target string, opts *imbot.SendMessageOptions) (*imbot.SendResult, error) {
	s.actions = opts.Actions
	return s.fakeSender.SendMessage(ctx, target, opts)
}

func (s *actionSender) Restate(_ context.Context, _ imbot.MessageRef, opts imbot.RestateOptions) error {
	s.restated = opts
	return s.restateErr
}

func TestSendActionsAndRestate(t *testing.T) {
	s := &actionSender{}
	c := New("bot", "telegram", s, nil, nil)
	id, err := c.SendActions(context.Background(), "42", "Approve?", []Button{{Label: "Allow", Payload: []string{"pj", "t", "r", "allow"}}})
	if err != nil || id != "m1" {
		t.Fatalf("SendActions = %q, %v", id, err)
	}
	if s.actions == nil || s.lastText != "Approve?" {
		t.Fatalf("buttons not sent: %+v", s.actions)
	}

	if err := c.Restate(context.Background(), "42", id, "Approve?\n\nAllowed."); err != nil {
		t.Fatal(err)
	}
	if s.restated.Text != "Approve?\n\nAllowed." || s.restated.Actions != nil {
		t.Fatalf("restate = %+v, want the text and no buttons", s.restated)
	}

	// A platform that cannot restate this message still hears the outcome.
	s.restateErr = errors.New("too old")
	s.lastText = ""
	if err := c.Restate(context.Background(), "42", id, "Denied."); err != nil {
		t.Fatal(err)
	}
	if s.lastText != "Denied." {
		t.Fatalf("fallback send = %q", s.lastText)
	}
}

func TestSendActionsWithoutButtons(t *testing.T) {
	// WeChat has no inline buttons; its chats get a line of text instead.
	c := New("bot", "weixin", &fakeSender{}, nil, nil)
	if _, err := c.SendActions(context.Background(), "42", "x", nil); !errors.Is(err, ErrNoButtons) {
		t.Fatalf("err = %v, want ErrNoButtons", err)
	}
}
