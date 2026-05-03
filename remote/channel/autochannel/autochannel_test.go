package autochannel

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/interaction"
)

func TestPromptPermissionPolicies(t *testing.T) {
	cases := []struct {
		name         string
		policy       string
		wantStatus   interaction.Status
		wantSelected string
	}{
		{"allow", DecisionAllow, interaction.StatusAnswered, "allow"},
		{"deny-explicit", DecisionDeny, interaction.StatusAnswered, "deny"},
		{"deny-default", "", interaction.StatusAnswered, "deny"},
		{"unknown-falls-back-to-deny", "garbage", interaction.StatusAnswered, "deny"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := New("auto", Policy{OnPermission: tc.policy}, nil)
			reply, err := c.Prompt(context.Background(), channel.Target{ChatID: "x"}, interaction.Interaction{
				ID:    "ix1",
				Kind:  interaction.KindConfirm,
				Title: "approve?",
			})
			if err != nil {
				t.Fatal(err)
			}
			if reply.Status != tc.wantStatus {
				t.Fatalf("status = %v, want %v", reply.Status, tc.wantStatus)
			}
			if reply.Selected != tc.wantSelected {
				t.Fatalf("selected = %q, want %q", reply.Selected, tc.wantSelected)
			}
		})
	}
}

func TestPromptQuestionPolicies(t *testing.T) {
	cases := []struct {
		name         string
		policy       string
		options      []interaction.Option
		wantStatus   interaction.Status
		wantSelected string
	}{
		{"auto-first-with-options", DecisionAutoFirst,
			[]interaction.Option{{Value: "a"}, {Value: "b"}},
			interaction.StatusAnswered, "a"},
		{"auto-first-no-options", DecisionAutoFirst, nil,
			interaction.StatusAnswered, ""},
		{"cancel-explicit", DecisionCancel, []interaction.Option{{Value: "a"}},
			interaction.StatusCancelled, ""},
		{"cancel-default", "", []interaction.Option{{Value: "a"}},
			interaction.StatusCancelled, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := New("auto", Policy{OnQuestion: tc.policy}, nil)
			reply, err := c.Prompt(context.Background(), channel.Target{ChatID: "x"}, interaction.Interaction{
				ID:      "ix2",
				Kind:    interaction.KindChoose,
				Options: tc.options,
			})
			if err != nil {
				t.Fatal(err)
			}
			if reply.Status != tc.wantStatus {
				t.Fatalf("status = %v, want %v", reply.Status, tc.wantStatus)
			}
			if reply.Selected != tc.wantSelected {
				t.Fatalf("selected = %q, want %q", reply.Selected, tc.wantSelected)
			}
		})
	}
}

func TestPromptCancelledContextReturnsError(t *testing.T) {
	c := New("auto", Policy{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Prompt(ctx, channel.Target{}, interaction.Interaction{ID: "ix"})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestSendInvokesSink(t *testing.T) {
	var mu sync.Mutex
	var captured []map[string]any
	c := New("auto", Policy{}, func(action string, fields map[string]any) {
		mu.Lock()
		defer mu.Unlock()
		fields["__action"] = action
		captured = append(captured, fields)
	})
	if err := c.Send(context.Background(), channel.Target{ChatID: "c1"}, interaction.Notification{
		Title: "hello",
		Body:  "world",
	}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(captured) != 1 {
		t.Fatalf("expected 1 sink call, got %d", len(captured))
	}
	got := captured[0]
	if got["chat_id"] != "c1" || got["title"] != "hello" || got["body"] != "world" {
		t.Fatalf("sink fields wrong: %+v", got)
	}
}

func TestConcurrentSendsAndPromptsRaceFree(t *testing.T) {
	var sinkCalls atomic.Int32
	c := New("auto", Policy{OnPermission: DecisionAllow}, func(string, map[string]any) {
		sinkCalls.Add(1)
	})
	const N = 50
	var wg sync.WaitGroup
	wg.Add(N * 2)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_ = c.Send(context.Background(), channel.Target{ChatID: "c"}, interaction.Notification{Body: "m"})
		}()
		go func() {
			defer wg.Done()
			_, _ = c.Prompt(context.Background(), channel.Target{ChatID: "c"}, interaction.Interaction{
				ID:   "ix",
				Kind: interaction.KindConfirm,
			})
		}()
	}
	wg.Wait()
	if got := sinkCalls.Load(); got != N*2 {
		t.Fatalf("sink calls = %d, want %d", got, N*2)
	}
}

func TestCapabilitiesAreAuto(t *testing.T) {
	c := New("auto", Policy{}, nil)
	caps := c.Capabilities()
	if caps.Buttons || caps.EditMessages || caps.Markdown {
		t.Fatalf("autochannel reports IM-style capabilities: %+v", caps)
	}
}

func TestImplementsChannelInterface(t *testing.T) {
	var _ channel.Channel = New("auto", Policy{}, nil)
}
