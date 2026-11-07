package project

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Each command is described once, and ChannelCommand only runs described ones.
func TestChatCommandsAreDescribedOnce(t *testing.T) {
	seen := map[string]bool{}
	orders := map[int]bool{}
	for _, c := range ChatCommands {
		assert.False(t, seen[c.Name], "duplicate %s", c.Name)
		assert.False(t, orders[c.Order], "duplicate order %d", c.Order)
		seen[c.Name], orders[c.Order] = true, true
		assert.True(t, isChatCommand(c.Name), c.Name)
		assert.True(t, strings.HasPrefix(c.Usage, "/"+c.Name), "usage %q starts with its command", c.Usage)
		assert.NotEmpty(t, c.Help, c.Name)
		assert.False(t, strings.HasSuffix(c.Help, "."), "%s: no final period", c.Name)
	}
	assert.False(t, isChatCommand("dir"), "folder commands are the host's")
}
