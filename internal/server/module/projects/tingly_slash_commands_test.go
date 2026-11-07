package projects

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/testenv"
	"github.com/tingly-dev/tingly-box/remote/access"
	"github.com/tingly-dev/tingly-box/remote/agent"
)

// Platform mention formats are simulated by sending that text through the
// Tingly transport with the chat type the platform's adapter sets. The
// harness cannot show what text the real Telegram, Slack, Discord, Feishu,
// WeCom, WhatsApp or DingTalk adapters deliver; those adapters do not run here.
var groupDecorations = []string{
	"/dir@tingly_bot", "@tingly_bot /dir", "@tingly_bot/dir", "<@U0BOT1> /dir", "<@!123456789> /dir",
	"@_user_1 /dir", " /dir", "／dir",
}

const needsPermission = "Any folder and files"

func (h *tinglyProjectsHarness) setPrivileged(t *testing.T, chat *testenv.Chat, allow bool) {
	t.Helper()
	effect := access.EffectDeny
	if allow {
		effect = access.EffectAllow
	}
	chats, err := h.stores.BotAccess().ListDirectChats(context.Background(), h.chosenID)
	require.NoError(t, err)
	for _, c := range chats {
		if c.ExternalChatID == chat.ChatID {
			require.NoError(t, h.stores.BotAccess().SetDirectChatPermission(context.Background(), h.chosenID, c.ID,
				access.CapabilityRemoteControl, access.ActionRemoteControlPrivileged, effect))
			return
		}
	}
	t.Fatalf("direct chat %q is not in the access store", chat.ChatID)
}

func (h *tinglyProjectsHarness) currentFolder(t *testing.T, chat *testenv.Chat) string {
	t.Helper()
	place, err := h.bridge.Place(agent.ChatRef{Platform: "tingly", ChatID: chat.ChatID})
	require.NoError(t, err)
	return place.Folder
}

// listNumber is the number a /dir or /project reply gave to the entry that
// contains needle.
func listNumber(t *testing.T, reply, needle string) string {
	t.Helper()
	for _, line := range strings.Split(reply, "\n") {
		if strings.Contains(line, needle) {
			if words := strings.Fields(line); len(words) > 0 {
				return strings.TrimSuffix(words[0], ".")
			}
		}
	}
	t.Fatalf("%q is not listed in %q", needle, reply)
	return ""
}

func newDirectSlashChat(t *testing.T, name string) (*tinglyProjectsHarness, *testenv.Chat) {
	h := newTinglyProjectsHarness(t)
	chat := h.env.NewUser(name).OpenDM(h.chosenID)
	chat.SendText("start")
	chat.WaitText(10*time.Second).AssertContains(t, "Harness reply: start")
	return h, chat
}

// A paired direct chat that may only start still chooses among the folders
// and projects the operator registered: /dir, /dir N, /project, /project N,
// the folder buttons and the menu's project button.
func TestTinglyDirectChatPicksRegisteredFoldersAndProjectsWithoutPrivileged(t *testing.T) {
	h, chat := newDirectSlashChat(t, "picker")
	first, second := realTempDir(t), realTempDir(t)
	h.call("POST", "/projects", map[string]any{"type": "folder", "path": first}, nil)
	h.call("POST", "/projects", map[string]any{"type": "folder", "path": second}, nil)
	h.call("POST", "/projects", map[string]any{"name": "Project Alpha"}, nil)

	chat.SendText("/dir")
	listed := chat.WaitText(10 * time.Second)
	listed.AssertContains(t, "Registered folders:").AssertContains(t, first).AssertContains(t, second)
	chat.SendText("/dir " + listNumber(t, listed.Text, second))
	chat.WaitText(10*time.Second).AssertContains(t, second)
	require.Equal(t, second, h.currentFolder(t, chat), "/dir N moved the chat")

	chat.SendCallback("old-message", "project:switch:"+first)
	chat.WaitText(10*time.Second).AssertContains(t, first)
	require.Equal(t, first, h.currentFolder(t, chat))
	chat.Drain()
	chat.SendCallback("old-message", "action:project")
	chat.WaitText(10*time.Second).AssertContains(t, "Registered folders:")

	chat.SendText("/project")
	projects := chat.WaitText(10 * time.Second)
	projects.AssertContains(t, "Project Alpha")
	chat.SendText("/project " + listNumber(t, projects.Text, "Project Alpha"))
	chat.WaitText(10*time.Second).AssertContains(t, "coordinator of Project Alpha")
}

// A free path, the directory browser and files need the permission. The chat
// is told which one instead of the command doing nothing, and it works once
// the permission is granted. /help follows the permission.
func TestTinglyDirectChatFreePathNeedsPrivilegedGrant(t *testing.T) {
	h, chat := newDirectSlashChat(t, "free-path")
	free := realTempDir(t)
	before := h.currentFolder(t, chat)

	for _, text := range []string{"/cd " + free, "/dir " + free, "  /cd   " + free + "  ", "／cd " + free, "/dir 0", "/dir 1a", "/cd"} {
		chat.Drain()
		chat.SendText(text)
		chat.WaitText(10*time.Second).AssertContains(t, needsPermission)
	}
	chat.Drain()
	chat.SendCallback("old-message", "action:bind")
	chat.WaitText(10*time.Second).AssertContains(t, needsPermission)
	chat.Drain()
	chat.SendMedia([]core.MediaAttachment{{Type: "document", URL: "http://example.invalid/a.txt", MimeType: "text/plain", Filename: "a.txt"}})
	chat.WaitText(10*time.Second).AssertContains(t, needsPermission)
	require.Equal(t, before, h.currentFolder(t, chat), "nothing moved the chat")

	chat.Drain()
	chat.SendText("/help")
	help := chat.WaitText(10*time.Second).AssertContains(t, "/dir [N]").AssertContains(t, "/project")
	require.NotContains(t, help.Text, "/cd")

	h.setPrivileged(t, chat, true)
	chat.Drain()
	chat.SendText("/cd " + free)
	chat.WaitText(10*time.Second).AssertContains(t, free)
	require.Equal(t, free, h.currentFolder(t, chat))
	other := realTempDir(t)
	chat.SendText("/dir   " + other)
	chat.WaitText(10*time.Second).AssertContains(t, other)
	require.Equal(t, other, h.currentFolder(t, chat))
	chat.Drain()
	chat.SendText("/help")
	chat.WaitText(10*time.Second).AssertContains(t, "/cd <path>").AssertContains(t, "/dir [N|path]")

	h.setPrivileged(t, chat, false)
	chat.Drain()
	chat.SendText("/cd " + free)
	chat.WaitText(10*time.Second).AssertContains(t, needsPermission)
	require.Equal(t, other, h.currentFolder(t, chat))
}

// In a direct chat only a plain command counts: whitespace and a full-width
// slash are fine, and a mention makes it ordinary text for the session.
func TestTinglyDirectChatReadsPlainCommandsOnly(t *testing.T) {
	_, chat := newDirectSlashChat(t, "plain")
	for _, text := range []string{"  /dir  ", "\t/dir\n", "／dir", "/DIR"} {
		chat.Drain()
		chat.SendText(text)
		chat.WaitText(10*time.Second).AssertContains(t, "Current folder")
	}
	for _, text := range []string{"@tingly_bot /dir", "<@U0BOT1>/dir", "@_user_1 /dir"} {
		chat.Drain()
		chat.SendText(text)
		reply := chat.WaitText(10 * time.Second)
		reply.AssertContains(t, "Harness reply: "+text)
		require.NotContains(t, reply.Text, "Current folder", "%q was read as a command", text)
	}
}

// newGroupSlashChat is a whitelisted group where alice is an authorized
// controller (start and approve, no privileged).
func newGroupSlashChat(t *testing.T) (h *tinglyProjectsHarness, chat *testenv.Chat, setPrivileged func(bool)) {
	t.Helper()
	h = newTinglyProjectsHarness(t)
	alice, bob := h.env.NewUser("alice"), h.env.NewUser("bob")
	chat = h.env.NewGroup("eng", alice, bob).Chat(h.chosenID)
	require.NoError(t, h.stores.RemoteChats().AddToWhitelist(chat.ChatID, "tingly", alice.ID))

	// A new group starts with no access; its first message only records it.
	ctx, store := context.Background(), h.stores.BotAccess()
	chat.SendText("hello")
	chat.ExpectNoEvent(300*time.Millisecond, tingly.EventSend)
	groups, err := store.ListGroups(ctx, h.chosenID)
	require.NoError(t, err)
	var group access.Group
	for _, g := range groups {
		if g.ExternalGroupID == chat.ChatID {
			group = g
		}
	}
	require.NotEmpty(t, group.ID, "the group was discovered by its first message")
	require.NoError(t, store.SetGroupCapability(ctx, h.chosenID, group.ID, access.CapabilityRemoteControl, access.EffectAllow))
	actor, err := store.AddGroupActor(ctx, h.chosenID, group.ID, alice.ID, "alice", "Controller")
	require.NoError(t, err)

	chat.SendText("start")
	chat.WaitText(10*time.Second).AssertContains(t, "Harness reply: start")
	setPrivileged = func(allow bool) {
		effect := access.EffectDeny
		if allow {
			effect = access.EffectAllow
		}
		require.NoError(t, store.SetGroupActorPermission(ctx, h.chosenID, group.ID, actor.ID,
			access.CapabilityRemoteControl, access.ActionRemoteControlPrivileged, effect))
	}
	return h, chat, setPrivileged
}

// In a group every folder and project command needs the permission, in every
// way the platforms write the command, and the decorated forms cannot get past
// the gate as ordinary text.
func TestTinglyGroupDecoratedFolderCommandsNeedPrivileged(t *testing.T) {
	h, chat, setPrivileged := newGroupSlashChat(t)
	registered := realTempDir(t)
	h.call("POST", "/projects", map[string]any{"type": "folder", "path": registered}, nil)
	h.call("POST", "/projects", map[string]any{"name": "Project Alpha"}, nil)
	before := h.currentFolder(t, chat)

	for _, text := range groupDecorations {
		chat.Drain()
		chat.SendText(text)
		reply := chat.WaitText(10 * time.Second)
		reply.AssertContains(t, needsPermission)
		require.NotContains(t, reply.Text, registered, "%q: the list was not shown", text)
	}
	for _, text := range []string{"/dir 1", "@tingly_bot /project", "<@U0BOT1>/project 1", "@tingly_bot /cd " + registered, "/dir " + registered} {
		chat.Drain()
		chat.SendText(text)
		chat.WaitText(10*time.Second).AssertContains(t, needsPermission)
	}
	chat.Drain()
	chat.SendCallback("old-message", "project:switch:"+registered)
	chat.WaitText(10*time.Second).AssertContains(t, needsPermission)
	require.Equal(t, before, h.currentFolder(t, chat), "no decorated form moved the group")

	chat.Drain()
	chat.SendText("@tingly_bot/help")
	help := chat.WaitText(10*time.Second).AssertContains(t, "/session")
	for _, hidden := range []string{"/cd", "/dir", "/project"} {
		require.NotContains(t, help.Text, hidden)
	}
	chat.Drain()
	chat.SendText("@tingly_bot what is in here?")
	chat.WaitText(10*time.Second).AssertContains(t, "Harness reply: @tingly_bot what is in here?")

	setPrivileged(true)
	for _, text := range groupDecorations {
		chat.Drain()
		chat.SendText(text)
		chat.WaitText(10*time.Second).AssertContains(t, "Registered folders:")
	}
	chat.Drain()
	chat.SendText("@tingly_bot /cd " + registered)
	chat.WaitText(10*time.Second).AssertContains(t, registered)
	require.Equal(t, registered, h.currentFolder(t, chat))
	chat.Drain()
	chat.SendText("/help@tingly_bot")
	chat.WaitText(10*time.Second).AssertContains(t, "/cd <path>").AssertContains(t, "/project")

	setPrivileged(false)
	chat.Drain()
	chat.SendText("@tingly_bot/dir")
	chat.WaitText(10*time.Second).AssertContains(t, needsPermission)
}
