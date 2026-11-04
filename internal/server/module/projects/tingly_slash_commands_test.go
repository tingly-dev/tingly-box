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
)

// What the harness can and cannot say about each platform's mention format.
// The Tingly transport carries the inbound text unchanged, so a platform's
// format is simulated by sending that text with the chat type the platform's
// adapter sets: Telegram "/dir@bot", Slack and Discord "<@U1>" / "<@!1>",
// Feishu and Lark "@_user_1", WeCom, WhatsApp and DingTalk "@name" or a
// leading space, Chinese keyboards a full-width slash. What it cannot show is
// whether each real adapter delivers exactly that text (for example whether
// DingTalk strips the @bot itself before the message arrives); those adapters
// are not run here.
var groupDecorations = []struct{ name, format string }{
	{"telegram suffix", "/dir@tingly_bot"},
	{"telegram suffix uppercase", "/DIR@Tingly_Bot"},
	{"mention then space", "@tingly_bot /dir"},
	{"mention no space", "@tingly_bot/dir"},
	{"slack mention", "<@U0BOT1> /dir"},
	{"slack mention no space", "<@U0BOT1>/dir"},
	{"discord mention", "<@!123456789> /dir"},
	{"feishu placeholder", "@_user_1 /dir"},
	{"feishu placeholder no space", "@_user_1/dir"},
	{"dingtalk leading space", " /dir"},
	{"padded", "   @tingly_bot    /dir   "},
	{"full-width slash", "／dir"},
}

func (h *tinglyProjectsHarness) directChatID(t *testing.T, chat *testenv.Chat) string {
	t.Helper()
	var list struct {
		Chats []struct {
			Chat struct {
				ID             string `json:"id"`
				ExternalChatID string `json:"external_chat_id"`
			} `json:"chat"`
		} `json:"chats"`
	}
	h.call("GET", "/bots/"+h.chosenID+"/chats", nil, &list)
	for _, c := range list.Chats {
		if c.Chat.ExternalChatID == chat.ChatID {
			return c.Chat.ID
		}
	}
	t.Fatalf("direct chat %q is not in the access list", chat.ChatID)
	return ""
}

func (h *tinglyProjectsHarness) setChatPrivileged(t *testing.T, chat *testenv.Chat, allow bool) {
	t.Helper()
	effect := "deny"
	if allow {
		effect = "allow"
	}
	h.call("PUT", "/bots/"+h.chosenID+"/chats/"+h.directChatID(t, chat)+"/permissions/remote_control/remote_control.privileged", map[string]any{"effect": effect}, nil)
}

func (h *tinglyProjectsHarness) currentFolder(t *testing.T, chat *testenv.Chat) string {
	t.Helper()
	folder, err := h.bridge.CurrentFolder(context.Background(), "tingly", chat.ChatID)
	require.NoError(t, err)
	return folder
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

const needsPermission = "Any folder and files"

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
	first, second := t.TempDir(), t.TempDir()
	h.call("POST", "/projects", map[string]any{"type": "folder", "path": first}, nil)
	h.call("POST", "/projects", map[string]any{"type": "folder", "path": second}, nil)
	var project struct {
		ID string `json:"id"`
	}
	h.call("POST", "/projects", map[string]any{"name": "Project Alpha"}, &project)

	chat.SendText("/dir")
	listed := chat.WaitText(10 * time.Second)
	listed.AssertContains(t, "Registered folders:").AssertContains(t, first).AssertContains(t, second)
	chat.SendText("/dir " + listNumber(t, listed.Text, second))
	chat.WaitText(10*time.Second).AssertContains(t, second)
	require.Equal(t, second, h.currentFolder(t, chat), "/dir N moved the chat")

	// The folder buttons and the menu's project button are the same pickers.
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
// the permission is granted through the access API the admin switch calls.
func TestTinglyDirectChatFreePathNeedsPrivilegedGrant(t *testing.T) {
	h, chat := newDirectSlashChat(t, "free-path")
	free := t.TempDir()
	before := h.currentFolder(t, chat)

	denied := []string{
		"/cd " + free, "/dir " + free, "/DIR " + free, "  /cd   " + free + "  ", "／cd " + free, " /cd " + free,
		"/dir 0", "/dir 1a", "/dir ~/x", "/cd",
	}
	for _, text := range denied {
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

	// /help offers only what the chat can run.
	chat.Drain()
	chat.SendText("/help")
	help := chat.WaitText(10*time.Second).AssertContains(t, "/dir [N]").AssertContains(t, "/project")
	require.NotContains(t, help.Text, "/cd")

	h.setChatPrivileged(t, chat, true)
	chat.Drain()
	chat.SendText("/cd " + free)
	chat.WaitText(10 * time.Second).AssertContains(t, free)
	require.Equal(t, free, h.currentFolder(t, chat))
	other := t.TempDir()
	chat.SendText("/dir   " + other)
	chat.WaitText(10 * time.Second).AssertContains(t, other)
	require.Equal(t, other, h.currentFolder(t, chat))
	chat.Drain()
	chat.SendText("/help")
	chat.WaitText(10*time.Second).AssertContains(t, "/cd <path>").AssertContains(t, "/dir [N|path]")

	// Turning the switch off again takes the command away at once.
	h.setChatPrivileged(t, chat, false)
	chat.Drain()
	chat.SendText("/cd " + free)
	chat.WaitText(10*time.Second).AssertContains(t, needsPermission)
	require.Equal(t, other, h.currentFolder(t, chat))
}

// In a direct chat only a plain command counts: extra, leading and trailing
// whitespace and a full-width slash are fine, and a mention makes it ordinary
// text for the session. Nothing in a direct chat needs an @.
func TestTinglyDirectChatReadsPlainCommandsOnly(t *testing.T) {
	h, chat := newDirectSlashChat(t, "plain")
	_ = h
	for _, text := range []string{"/dir", "  /dir  ", "\t/dir\n", "/DIR", "／dir", " /dir", "​/dir", "/dir   "} {
		chat.Drain()
		chat.SendText(text)
		chat.WaitText(10*time.Second).AssertContains(t, "Current folder")
	}

	for _, text := range []string{"@tingly_bot /dir", "@tingly_bot/dir", "<@U0BOT1> /dir", "<@U0BOT1>/dir", "@_user_1 /dir", "@_user_1/dir"} {
		chat.Drain()
		chat.SendText(text)
		reply := chat.WaitText(10 * time.Second)
		reply.AssertContains(t, "Harness reply: "+text)
		require.NotContains(t, reply.Text, "Current folder", "%q was read as a command", text)
	}

	// The bot's suffix is a group thing: there it is no command either.
	chat.Drain()
	chat.SendText("/dir@tingly_bot")
	chat.WaitText(10*time.Second).AssertContains(t, "Unknown command /dir@tingly_bot")
}

// newGroupSlashChat is a whitelisted group where alice is an authorized
// controller (start and approve, no privileged), set up through the access API.
func newGroupSlashChat(t *testing.T) (*tinglyProjectsHarness, *testenv.Chat, *testenv.User, string, string) {
	t.Helper()
	h := newTinglyProjectsHarness(t)
	alice, bob := h.env.NewUser("alice"), h.env.NewUser("bob")
	chat := h.env.NewGroup("eng", alice, bob).Chat(h.chosenID)
	require.NoError(t, h.stores.RemoteChats().AddToWhitelist(chat.ChatID, "tingly", alice.ID))

	// A new group starts with no access: its first message is dropped, and the
	// group is recorded so the operator can allow it.
	chat.SendText("hello")
	chat.ExpectNoEvent(300*time.Millisecond, tingly.EventSend)
	var groups struct {
		Groups []struct {
			ID              string `json:"id"`
			ExternalGroupID string `json:"external_group_id"`
		} `json:"groups"`
	}
	h.call("GET", "/bots/"+h.chosenID+"/groups", nil, &groups)
	groupID := ""
	for _, g := range groups.Groups {
		if g.ExternalGroupID == chat.ChatID {
			groupID = g.ID
		}
	}
	require.NotEmpty(t, groupID, "the group was discovered by its first message")
	h.call("PUT", "/bots/"+h.chosenID+"/groups/"+groupID+"/capabilities/remote_control", map[string]any{"effect": "allow"}, nil)
	var actor struct {
		ID string `json:"id"`
	}
	h.call("PUT", "/bots/"+h.chosenID+"/groups/"+groupID+"/actors/"+alice.ID, map[string]any{"external_actor_id": alice.ID, "display_name": "alice", "label": "Controller"}, &actor)
	require.NotEmpty(t, actor.ID)

	chat.SendText("start")
	chat.WaitText(10*time.Second).AssertContains(t, "Harness reply: start")
	return h, chat, bob, groupID, actor.ID
}

func (h *tinglyProjectsHarness) setActorPrivileged(groupID, actorID string, allow bool) {
	effect := "deny"
	if allow {
		effect = "allow"
	}
	h.call("PUT", "/bots/"+h.chosenID+"/groups/"+groupID+"/actors/"+actorID+"/permissions/remote_control/remote_control.privileged", map[string]any{"effect": effect}, nil)
}

// In a group every folder and project command needs the permission, in every
// way the platforms write the command, and the decorated forms cannot get past
// the gate as ordinary text.
func TestTinglyGroupDecoratedFolderCommandsNeedPrivileged(t *testing.T) {
	h, chat, bob, groupID, actorID := newGroupSlashChat(t)
	registered := t.TempDir()
	h.call("POST", "/projects", map[string]any{"type": "folder", "path": registered}, nil)
	h.call("POST", "/projects", map[string]any{"name": "Project Alpha"}, nil)
	before := h.currentFolder(t, chat)

	for _, d := range groupDecorations {
		chat.Drain()
		chat.SendText(d.format)
		reply := chat.WaitText(10 * time.Second)
		reply.AssertContains(t, needsPermission)
		require.NotContains(t, reply.Text, registered, "%s: the list was not shown", d.name)
	}
	for _, text := range []string{"/dir 1", "/dir@tingly_bot 1", "@tingly_bot /project", "<@U0BOT1>/project 1", "/project@tingly_bot", "@tingly_bot /cd " + registered, "@_user_1/cd " + registered, "/dir " + registered} {
		chat.Drain()
		chat.SendText(text)
		chat.WaitText(10*time.Second).AssertContains(t, needsPermission)
	}
	chat.Drain()
	chat.SendCallback("old-message", "project:switch:"+registered)
	chat.WaitText(10*time.Second).AssertContains(t, needsPermission)
	chat.Drain()
	chat.SendCallback("old-message", "action:project")
	chat.WaitText(10*time.Second).AssertContains(t, needsPermission)
	require.Equal(t, before, h.currentFolder(t, chat), "no decorated form moved the group")

	// A group member nobody authorized is not told anything.
	chat.Drain()
	chat.SendTextAs(bob, "/dir")
	chat.SendTextAs(bob, "@tingly_bot /cd "+registered)
	chat.ExpectNoEvent(300*time.Millisecond, tingly.EventSend)

	// /help lists what the group member can run: none of the three.
	for _, text := range []string{"/help", "/help@tingly_bot", "@tingly_bot /help", "@_user_1/help"} {
		chat.Drain()
		chat.SendText(text)
		help := chat.WaitText(10*time.Second).AssertContains(t, "/session")
		for _, hidden := range []string{"/cd", "/dir", "/project"} {
			require.NotContains(t, help.Text, hidden, "%q: %s is not offered", text, hidden)
		}
	}

	// A mention in front of ordinary text is still a message for the session.
	chat.Drain()
	chat.SendText("@tingly_bot what is in here?")
	chat.WaitText(10 * time.Second).AssertContains(t, "Harness reply: @tingly_bot what is in here?")

	// With the permission every decorated form works, through the API the
	// admin switch calls, and taking it away stops them again.
	h.setActorPrivileged(groupID, actorID, true)
	for _, d := range groupDecorations {
		chat.Drain()
		chat.SendText(d.format)
		chat.WaitText(10*time.Second).AssertContains(t, "Registered folders:")
	}
	for _, text := range []string{"@tingly_bot /cd " + registered, "/cd@tingly_bot " + registered, "<@!123456789>/dir " + registered, "@_user_1 /project"} {
		chat.Drain()
		chat.SendText(text)
		reply := chat.WaitText(10 * time.Second)
		require.NotContains(t, reply.Text, needsPermission, text)
	}
	require.Equal(t, registered, h.currentFolder(t, chat))
	chat.Drain()
	chat.SendText("/help@tingly_bot")
	chat.WaitText(10*time.Second).AssertContains(t, "/cd <path>").AssertContains(t, "/project")

	h.setActorPrivileged(groupID, actorID, false)
	chat.Drain()
	chat.SendText("@tingly_bot/dir")
	chat.WaitText(10*time.Second).AssertContains(t, needsPermission)
}
