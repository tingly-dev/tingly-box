package chat

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/imbot"
)

func TestShortcutHelpAndRegistration(t *testing.T) {
	registry := imbot.NewCommandRegistry()
	require.NoError(t, RegisterBuiltinCommands(registry, nil))
	for _, pair := range [][2]string{{"h", "help"}, {"d", "dir"}, {"proj", "project"}, {"sess", "session"}, {"n", "new"}, {"sto", "stop"}, {"cl", "clear"}, {"stat", "status"}, {"v", "verbose"}} {
		cmd, ok := registry.Get(pair[0])
		require.True(t, ok, pair[0])
		assert.Equal(t, pair[1], cmd.Name)
		assert.Contains(t, helpText("u", "c", true, false), "/"+pair[0])
	}
	group := helpText("u", "c", false, true)
	assert.NotContains(t, group, "/d ")
	assert.NotContains(t, group, "/proj")
	direct := helpText("u", "c", false, false)
	assert.Contains(t, direct, "/d")
	assert.Contains(t, direct, "/proj")
}

func TestHelpText(t *testing.T) {
	const tail = `@N <message>: say one thing to session #N

Your ID: u1
Chat ID: c1`
	session := strings.Join([]string{
		"/session [N] (/sess): List the sessions here, or switch to #N",
		"/new [message] (/n): Start a new session",
		"/stop [#N] (/sto): Stop the running session",
		"/clear (/cl): Stop, and start a new session with the next message",
		"/status (/stat): Where this chat is and how its session runs",
		"/verbose on|off (/v): Also see each step, not only the reply",
		"/yolo: Use every tool without asking, or back",
	}, "\n")
	intro := "Send a message to talk to Claude Code.\n/h: Shortcut for /help"

	tests := []struct {
		name       string
		privileged bool
		group      bool
		want       string
	}{
		{"privileged direct", true, false, strings.Join([]string{intro,
			"/cd <path>: Work in another folder",
			"/dir [N|path] (/d): List registered folders, or switch to one",
			"/project [N] (/proj): List projects, or switch to a project's coordinator",
			session, tail}, "\n")},
		{"privileged group", true, true, strings.Join([]string{intro,
			"/cd <path>: Work in another folder",
			"/dir [N|path] (/d): List registered folders, or switch to one",
			"/project [N] (/proj): List projects, or switch to a project's coordinator",
			session, tail}, "\n")},
		{"direct picker only", false, false, strings.Join([]string{intro,
			"/dir [N] (/d): List registered folders, or switch to one",
			"/project [N] (/proj): List projects, or switch to a project's coordinator",
			session, tail}, "\n")},
		{"group without permission", false, true, strings.Join([]string{intro, session, tail}, "\n")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, helpText("u1", "c1", tt.privileged, tt.group))
		})
	}
}

// The registry is what a platform shows as its command menu.
func TestBuiltinCommandMenu(t *testing.T) {
	registry := imbot.NewCommandRegistry()
	require.NoError(t, RegisterBuiltinCommands(registry, nil))

	var got []string
	for _, c := range registry.All() {
		got = append(got, fmt.Sprintf("%s|%s|%d|hidden=%v|%s", c.Name, c.Category, c.Priority, c.Hidden, c.Description))
	}
	assert.Equal(t, []string{
		"help|system|100|hidden=false|Show what this chat can do",
		"pair|system|95|hidden=true|Pair this chat with the bot using the operator's pairing code",
		"cd|project|90|hidden=false|Work in another folder",
		"dir|project|85|hidden=false|List registered folders, or switch to one",
		"project|project|84|hidden=false|List projects, or switch to a project's coordinator",
		"session|session|80|hidden=false|List the sessions here, or switch to #N",
		"new|session|79|hidden=false|Start a new session",
		"stop|session|78|hidden=false|Stop the running session",
		"clear|session|77|hidden=false|Stop, and start a new session with the next message",
		"status|session|76|hidden=false|Where this chat is and how its session runs",
		"verbose|session|75|hidden=false|Also see each step, not only the reply",
		"yolo|session|74|hidden=false|Use every tool without asking, or back",
		"join|system|30|hidden=true|Add group to whitelist (Telegram only)",
	}, got)
}

// The menu and /help cannot disagree: every listed command is registered with
// the description /help shows, at the position /help lists it.
func TestMenuAndHelpListTheSameCommands(t *testing.T) {
	registry := imbot.NewCommandRegistry()
	require.NoError(t, RegisterBuiltinCommands(registry, nil))

	help := helpText("u1", "c1", true, false)
	for _, spec := range commandSpecs() {
		cmd, ok := registry.Get(spec.Name)
		require.True(t, ok, "%s is registered", spec.Name)
		assert.Equal(t, spec.Help, cmd.Description, spec.Name)
		assert.Equal(t, spec.Order, cmd.Priority, spec.Name)
		usage, ok := spec.usageFor(true, false)
		require.True(t, ok)
		assert.Contains(t, help, usage, spec.Name)
		assert.Contains(t, help, ": "+spec.Help, spec.Name)
	}
}
