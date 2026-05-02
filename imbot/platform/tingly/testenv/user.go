package testenv

import (
	"github.com/tingly-dev/tingly-box/imbot/core"
)

// User is a synthetic IM user. Its ID is what the production bot sees in
// msg.Sender.ID — production handlers compare/store this so it must be
// stable for the whole test.
type User struct {
	env *TestEnv

	ID          string
	Username    string
	DisplayName string
}

// NewUser creates a user with deterministic identity fields based on the
// given name.
func (e *TestEnv) NewUser(name string) *User {
	return &User{
		env:         e,
		ID:          name + "-id",
		Username:    name,
		DisplayName: name,
	}
}

// OpenDM opens (or returns) a direct-message chat between the user and the
// given bot.
func (u *User) OpenDM(botUUID string) *Chat {
	chatID := "dm:" + u.ID
	return u.env.openChat(botUUID, chatID, core.ChatTypeDirect, []*User{u}, u)
}

// Sender converts the user into the core.Sender struct expected by the
// imbot platform layer.
func (u *User) Sender() core.Sender {
	return core.Sender{
		ID:          u.ID,
		Username:    u.Username,
		DisplayName: u.DisplayName,
	}
}

// Group is a multi-user chat. The first member acts as the "primary"
// sender for SendText / SendCallback (without an explicit As variant).
type Group struct {
	env *TestEnv

	ChatID  string
	Title   string
	Members []*User
}

// NewGroup creates a group chat populated with the given members.
func (e *TestEnv) NewGroup(title string, members ...*User) *Group {
	return &Group{
		env:     e,
		ChatID:  "group:" + e.nextID("g"),
		Title:   title,
		Members: members,
	}
}

// Chat returns the chat handle for this group at the given bot.
func (g *Group) Chat(botUUID string) *Chat {
	var primary *User
	if len(g.Members) > 0 {
		primary = g.Members[0]
	}
	return g.env.openChat(botUUID, g.ChatID, core.ChatTypeGroup, g.Members, primary)
}
