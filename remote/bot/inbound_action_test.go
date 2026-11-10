package bot_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/remote/access"
	"github.com/tingly-dev/tingly-box/remote/bot"
)

func TestDecoratedShortcutAuthorization(t *testing.T) {
	for _, tt := range []struct {
		text  string
		group bool
		want  access.ActionName
	}{
		{"/d /work", false, access.ActionRemoteControlPrivileged},
		{"/d 2", false, access.ActionRemoteControlStart},
		{"/d@bot 2", true, access.ActionRemoteControlPrivileged},
		{"@bot /p 2", true, access.ActionRemoteControlPrivileged},
		{"/p@bot 2", true, access.ActionRemoteControlPrivileged},
		{"/p 2", false, access.ActionRemoteControlStart},
		{"/p@bot 2", false, access.ActionRemoteControlStart},
		{"<@U123> /d /work", true, access.ActionRemoteControlPrivileged},
		{"/d@bot /work", false, access.ActionRemoteControlStart},
		{"@bot /d /work", false, access.ActionRemoteControlStart},
	} {
		msg := imbot.Message{ChatType: imbot.ChatTypeDirect, Content: imbot.NewTextContent(tt.text)}
		if tt.group {
			msg.ChatType = imbot.ChatTypeGroup
		}
		_, got := bot.InboundAction(msg, "", "")
		require.Equal(t, tt.want, got, tt.text)
	}
}

func TestShortcutAuthorizationParity(t *testing.T) {
	for _, pair := range [][2]string{{"d", "dir"}, {"p", "project"}, {"sess", "session"}, {"n", "new"}, {"sp", "stop"}, {"cr", "clear"}, {"stat", "status"}, {"v", "verbose"}} {
		for _, group := range []bool{false, true} {
			for _, arg := range []string{"", " 2", " 0", " /some path"} {
				for _, pending := range []access.ActionName{"", access.ActionRemoteControlApprove} {
					msg := imbot.Message{ChatType: imbot.ChatTypeDirect}
					if group {
						msg.ChatType = imbot.ChatTypeGroup
					}
					msg.Content = imbot.NewTextContent("/" + pair[1] + arg)
					wantCapability, wantAction := bot.InboundAction(msg, access.CapabilityRemoteControl, pending)
					msg.Content = imbot.NewTextContent("/" + pair[0] + arg)
					gotCapability, gotAction := bot.InboundAction(msg, access.CapabilityRemoteControl, pending)
					require.Equal(t, wantCapability, gotCapability, "%s group=%v arg=%q pending=%s", pair[0], group, arg, pending)
					require.Equal(t, wantAction, gotAction, "%s group=%v arg=%q pending=%s", pair[0], group, arg, pending)
				}
			}
		}
	}
}

func TestHelpShortcutAuthorizationParity(t *testing.T) {
	for _, group := range []bool{false, true} {
		for _, pending := range []access.ActionName{"", access.ActionRemoteControlApprove} {
			msg := imbot.Message{ChatType: imbot.ChatTypeDirect}
			if group {
				msg.ChatType = imbot.ChatTypeGroup
			}
			msg.Content = imbot.NewTextContent("/help extra")
			wantCapability, wantAction := bot.InboundAction(msg, access.CapabilityRemoteControl, pending)
			msg.Content = imbot.NewTextContent("/h extra")
			gotCapability, gotAction := bot.InboundAction(msg, access.CapabilityRemoteControl, pending)
			require.Equal(t, wantCapability, gotCapability)
			require.Equal(t, wantAction, gotAction)
		}
	}
}

// Choosing among registered folders and projects needs start; a free path and
// the directory browser need privileged; in a group every one of them does.
// Only a group strips mentions and "/cmd@bot", so a mention before a command in
// a direct chat is ordinary text.
func TestInboundAction(t *testing.T) {
	const start, privileged = access.ActionRemoteControlStart, access.ActionRemoteControlPrivileged
	msg := func(group bool, text string, segments ...string) imbot.Message {
		m := imbot.Message{ChatType: imbot.ChatTypeDirect, Content: imbot.NewTextContent(text)}
		if group {
			m.ChatType = imbot.ChatTypeGroup
		}
		if len(segments) > 0 {
			m.Payload = imbot.NewPayload(segments...)
		}
		return m
	}
	file := imbot.NewMediaContent([]imbot.MediaAttachment{{URL: "x"}}, "")
	direct, group := false, true
	for _, tt := range []struct {
		name string
		msg  imbot.Message
		want access.ActionName
	}{
		{"/dir", msg(direct, "/dir"), start},
		{"/DIR  2 padded", msg(direct, "  /DIR   2  "), start},
		{"full-width /dir", msg(direct, "\uff0fdir"), start},
		{"/project 1", msg(direct, "/project 1"), start},
		{"/dir path", msg(direct, "/dir /tmp"), privileged},
		{"/dir 0", msg(direct, "/dir 0"), privileged},
		{"/dir 1a", msg(direct, "/dir 1a"), privileged},
		{"/cd", msg(direct, "/cd /tmp"), privileged},
		{"/status", msg(direct, "/status"), start},
		{"/directory", msg(direct, "/directory"), start},
		{"mention is text", msg(direct, "@tingly_bot /cd /tmp"), start},
		{"bot suffix is not stripped", msg(direct, "/cd@tingly_bot /tmp"), start},
		{"folder button", msg(direct, "", "project", "switch", "/tmp"), start},
		{"menu project button", msg(direct, "", "action", "project"), start},
		{"menu bind button", msg(direct, "", "action", "bind"), privileged},
		{"browser button", msg(direct, "", "bind", "dir", "x"), privileged},
		{"direct file", func() imbot.Message { m := msg(direct, ""); m.Content = file; return m }(), privileged},

		{"group /dir", msg(group, "/dir"), privileged},
		{"group /dir 1", msg(group, "/dir 1"), privileged},
		{"group /project", msg(group, "/project"), privileged},
		{"group suffix", msg(group, "/dir@tingly_bot 1"), privileged},
		{"group mention", msg(group, "@tingly_bot /dir"), privileged},
		{"group mention no space", msg(group, "@tingly_bot/cd /tmp"), privileged},
		{"group slack mention", msg(group, "<@U123>/project"), privileged},
		{"group feishu mention", msg(group, "@_user_1 /dir"), privileged},
		{"group full-width", msg(group, "\uff0fdir"), privileged},
		{"group /status", msg(group, "@tingly_bot /status"), start},
		{"group folder button", msg(group, "", "project", "switch", "/tmp"), privileged},
		{"group menu project button", msg(group, "", "action", "project"), privileged},
	} {
		_, got := bot.InboundAction(tt.msg, "", "")
		require.Equal(t, tt.want, got, tt.name)
		// A pending approval never changes what a folder command needs.
		_, got = bot.InboundAction(tt.msg, access.CapabilityRemoteControl, access.ActionRemoteControlApprove)
		if tt.want == privileged || tt.msg.IsCallback() {
			require.Equal(t, tt.want, got, tt.name+" with a pending approval")
		}
	}
}
