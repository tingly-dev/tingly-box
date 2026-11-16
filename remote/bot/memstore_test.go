package bot_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/remote/bot"
)

func TestMemoryChatStore(t *testing.T) {
	s := bot.NewMemoryChatStore()

	got, err := s.GetChat("c1")
	require.NoError(t, err)
	assert.Nil(t, got, "a missing chat is (nil, nil)")

	c, err := s.GetOrCreateChat("c1", "telegram")
	require.NoError(t, err)
	assert.Equal(t, "telegram", c.Platform)
	_, err = s.GetOrCreateChat("c1", "slack")
	assert.Error(t, err, "a chat of another platform is refused")

	c.IsWhitelisted = true // a returned chat is a copy
	assert.False(t, s.IsWhitelisted("c1"))
	require.NoError(t, s.AddToWhitelist("c1", "telegram", "owner"))
	assert.True(t, s.IsWhitelisted("c1"))

	require.NoError(t, s.SetPaired("c1", "telegram", "bot-1", "u1"))
	assert.True(t, s.IsChatPaired("c1", "bot-1"))
	assert.False(t, s.IsChatPaired("c1", "bot-2"))
	require.NoError(t, s.ClearPaired("c1"))
	assert.False(t, s.IsChatPaired("c1", "bot-1"))
	assert.True(t, s.IsWhitelisted("c1"), "clearing the pairing keeps the rest")

	require.NoError(t, s.BindProject("c2", "telegram", "/a", "owner"))
	require.NoError(t, s.RememberFolder("c2", "telegram", "/b", "owner"))
	path, ok, _ := s.GetProjectPath("c2")
	assert.True(t, ok)
	assert.Equal(t, "/a", path, "RememberFolder leaves ProjectPath alone")
	hist, _ := s.ListChatProjectPaths("c2")
	assert.Equal(t, []string{"/b", "/a"}, hist)

	require.NoError(t, s.SetChatDisabled("c1", true))
	assert.True(t, s.IsChatDisabled("c1"))
	visible, _ := s.ListChats("telegram", false)
	assert.Len(t, visible, 1, "disabled chats are hidden")
	all, _ := s.ListChats("telegram", true)
	assert.Len(t, all, 2)

	require.NoError(t, s.DeleteChat("c1"))
	gone, _ := s.GetChat("c1")
	assert.Nil(t, gone)
	assert.NoError(t, s.DeleteChat("c1"), "deleting a missing chat is a no-op")
}
