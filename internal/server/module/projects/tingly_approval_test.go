package projects

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/testenv"
	"github.com/tingly-dev/tingly-box/remote/access"
)

// waitTextWith returns the next text of the chat that contains substr, setting
// aside the chatter around it (coordinator replies, thread reports).
func waitTextWith(t *testing.T, chat *testenv.Chat, substr string) *testenv.OutEvent {
	t.Helper()
	for range 12 {
		if e := chat.WaitText(10 * time.Second); strings.Contains(e.Text, substr) {
			return e
		}
	}
	t.Fatalf("no text containing %q", substr)
	return nil
}

// projectChatAskedByAWorker binds a chat to a project's coordinator, has the
// coordinator start a quiet worker (#1) and then one that asks for
// permission (#2, task), and returns the prompt the chat got. The chat then
// moves to #1 with /session 1, so it no longer follows the thread that asked,
// as happens when a chat follows another thread while a prompt waits.
func projectChatAskedByAWorker(t *testing.T, h *tinglyProjectsHarness, name, task string) (*testenv.Chat, *testenv.OutEvent) {
	t.Helper()
	chat := h.env.NewUser(name).OpenDM(h.chosenID)
	bindCoordinator(t, h, name+" project", chat.ChatID)
	chat.SendText("start worker write docs")
	waitTextWith(t, chat, "Worker started")
	chat.SendText("start worker " + task)
	var prompt *testenv.OutEvent
	for range 6 {
		if e := chat.WaitText(15 * time.Second); len(e.Buttons) > 0 {
			prompt = e
			break
		}
	}
	require.NotNil(t, prompt, "the worker's approval must reach the project's chat")
	prompt.AssertContains(t, "Approve: Run `echo harness`")
	chat.SendText("/session 1")
	waitTextWith(t, chat, "This chat now talks to")
	return chat, prompt
}

// A chat bound to a project approves what one of its threads asks, also when
// the chat has moved on to follow another thread.
func TestTinglyProjectChatApprovesAThreadsRequest(t *testing.T) {
	h := newTinglyProjectsHarness(t)
	chat, prompt := projectChatAskedByAWorker(t, h, "approver", "request approval")
	allow, ok := prompt.ButtonByLabel("Allow")
	require.True(t, ok)

	allow.Click()
	chat.WaitRestate(prompt.MessageID, 10*time.Second).AssertContains(t, "Allowed in a chat.")

	// A second press is too late and says so.
	allow.Click()
	waitTextWith(t, chat, "This was already answered.")
}

func TestTinglyProjectChatDeniesAThreadsRequest(t *testing.T) {
	h := newTinglyProjectsHarness(t)
	chat, prompt := projectChatAskedByAWorker(t, h, "denier", "request denial")
	deny, ok := prompt.ButtonByLabel("Deny")
	require.True(t, ok)

	deny.Click()
	chat.WaitRestate(prompt.MessageID, 10*time.Second).AssertContains(t, "Denied in a chat.")
}

// The chat can also answer the request in words.
func TestTinglyProjectChatAnswersAThreadsRequestInWords(t *testing.T) {
	h := newTinglyProjectsHarness(t)
	chat, prompt := projectChatAskedByAWorker(t, h, "talker", "request denial")

	chat.SendText("deny")
	chat.WaitRestate(prompt.MessageID, 10*time.Second).AssertContains(t, "Denied in a chat.")
}

// The chat follows a sibling before a worker asks, not after the prompt arrives.
func TestTinglyProjectReceivesApprovalAfterSwitch(t *testing.T) {
	h := newTinglyProjectsHarness(t)
	chat := h.env.NewUser("switched").OpenDM(h.chosenID)
	_, main := bindCoordinator(t, h, "switched project", chat.ChatID)
	chat.SendText("start worker write docs")
	waitTextWith(t, chat, "Worker started")
	chat.SendText("/session 1")
	waitTextWith(t, chat, "This chat now talks to")
	// The coordinator starts the sibling via the web while IM follows #1.
	h.call("POST", "/threads/"+main+"/messages", map[string]any{"content": "start worker request approval"}, nil)
	prompt := waitTextWith(t, chat, "Approve: Run `echo harness`")
	allow, ok := prompt.ButtonByLabel("Allow")
	require.True(t, ok)
	allow.Click()
	chat.WaitRestate(prompt.MessageID, 10*time.Second).AssertContains(t, "Allowed in a chat.")
}

// Text answers must require the same approval permission as buttons.
func TestTinglyProjectTextApprovalRequiresApprove(t *testing.T) {
	h := newTinglyProjectsHarness(t)
	chat, prompt := projectChatAskedByAWorker(t, h, "restricted", "request approval")
	chats, err := h.stores.BotAccess().ListDirectChats(context.Background(), h.chosenID)
	require.NoError(t, err)
	require.Len(t, chats, 1)
	require.NoError(t, h.stores.BotAccess().SetDirectChatPermission(context.Background(), h.chosenID, chats[0].ID,
		access.CapabilityRemoteControl, access.ActionRemoteControlApprove, access.EffectDeny))
	chat.SendText("allow")
	waitTextWith(t, chat, "not authorized")
	chat.ExpectNoEvent(300*time.Millisecond, tingly.EventRestate)
	require.NoError(t, h.stores.BotAccess().SetDirectChatPermission(context.Background(), h.chosenID, chats[0].ID,
		access.CapabilityRemoteControl, access.ActionRemoteControlApprove, access.EffectAllow))
	chat.SendText("allow")
	chat.WaitRestate(prompt.MessageID, 10*time.Second).AssertContains(t, "Allowed in a chat.")
}

func TestTinglyButtonlessTextApprovalRequiresApprove(t *testing.T) {
	h := newTinglyProjectsHarness(t)
	orig, ok := h.registry.Get(h.chosenID)
	require.True(t, ok)
	h.registry.Register(buttonless{orig})
	chat := h.env.NewUser("buttonless-restricted").OpenDM(h.chosenID)
	bindCoordinator(t, h, "buttonless project", chat.ChatID)
	chat.SendText("start worker request approval")
	prompt := waitTextWith(t, chat, "Approve: Run `echo harness`")
	require.Empty(t, prompt.Buttons)
	chats, err := h.stores.BotAccess().ListDirectChats(context.Background(), h.chosenID)
	require.NoError(t, err)
	require.Len(t, chats, 1)
	require.NoError(t, h.stores.BotAccess().SetDirectChatPermission(context.Background(), h.chosenID, chats[0].ID,
		access.CapabilityRemoteControl, access.ActionRemoteControlApprove, access.EffectDeny))
	chat.SendText("allow")
	waitTextWith(t, chat, "not authorized")
	require.NoError(t, h.stores.BotAccess().SetDirectChatPermission(context.Background(), h.chosenID, chats[0].ID,
		access.CapabilityRemoteControl, access.ActionRemoteControlApprove, access.EffectAllow))
	chat.SendText("allow")
	waitTextWith(t, chat, "request approval approved")
}

// A chat bound to another project cannot answer the request, and the request
// stays open for the chat that may.
func TestTinglyChatOfAnotherProjectCannotApproveAThreadsRequest(t *testing.T) {
	h := newTinglyProjectsHarness(t)
	chat, prompt := projectChatAskedByAWorker(t, h, "owner", "request approval")
	stranger := h.env.NewUser("stranger").OpenDM(h.chosenID)
	bindCoordinator(t, h, "Other project", stranger.ChatID)
	allow, ok := prompt.ButtonByLabel("Allow")
	require.True(t, ok)

	stranger.SendCallback(prompt.MessageID, allow.CallbackData)
	stranger.WaitText(10*time.Second).AssertContains(t, "Could not answer")
	chat.ExpectNoEvent(300*time.Millisecond, tingly.EventRestate)

	allow.Click()
	chat.WaitRestate(prompt.MessageID, 10*time.Second).AssertContains(t, "Allowed in a chat.")
}
