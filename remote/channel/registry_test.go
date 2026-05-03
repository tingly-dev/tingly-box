package channel

import (
	"context"
	"testing"

	"github.com/tingly-dev/tingly-box/remote/interaction"
)

type fakeChannel struct {
	id       string
	platform string
}

func (f *fakeChannel) ID() string                 { return f.id }
func (f *fakeChannel) Platform() string           { return f.platform }
func (f *fakeChannel) Capabilities() Capabilities { return Capabilities{} }
func (f *fakeChannel) Send(context.Context, Target, interaction.Notification) error {
	return nil
}
func (f *fakeChannel) Prompt(context.Context, Target, interaction.Interaction) (interaction.Reply, error) {
	return interaction.Reply{}, nil
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	c := &fakeChannel{id: "bot-1", platform: "telegram"}
	r.Register(c)
	got, ok := r.Get("bot-1")
	if !ok || got.ID() != "bot-1" {
		t.Fatalf("Get failed: %+v ok=%v", got, ok)
	}
}

func TestRegistryUnregister(t *testing.T) {
	r := NewRegistry()
	c := &fakeChannel{id: "bot-1"}
	r.Register(c)
	r.Unregister("bot-1")
	if _, ok := r.Get("bot-1"); ok {
		t.Fatal("channel should be removed after Unregister")
	}
}

func TestRegistryIgnoresNilOrEmpty(t *testing.T) {
	r := NewRegistry()
	r.Register(nil)
	r.Register(&fakeChannel{id: ""})
	if r.Len() != 0 {
		t.Fatalf("expected 0 channels, got %d", r.Len())
	}
}

func TestRegistryReplace(t *testing.T) {
	r := NewRegistry()
	r.Register(&fakeChannel{id: "x", platform: "telegram"})
	r.Register(&fakeChannel{id: "x", platform: "feishu"})
	got, _ := r.Get("x")
	if got.Platform() != "feishu" {
		t.Fatalf("expected replacement, got platform=%s", got.Platform())
	}
}
