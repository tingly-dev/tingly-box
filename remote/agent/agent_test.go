package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommandsAreDescribedOnce(t *testing.T) {
	seen := map[string]bool{}
	orders := map[int]bool{}
	for _, c := range Commands() {
		assert.False(t, seen[c.Name], "duplicate %s", c.Name)
		assert.False(t, orders[c.Order], "duplicate order %d", c.Order)
		seen[c.Name], orders[c.Order] = true, true
		assert.True(t, IsCommand(c.Name), c.Name)
		assert.True(t, strings.HasPrefix(c.Usage, "/"+c.Name), "usage %q starts with its command", c.Usage)
		assert.NotEmpty(t, c.Help, c.Name)
		assert.False(t, strings.HasSuffix(c.Help, "."), "%s: no final period", c.Name)
	}
	assert.False(t, IsCommand("dir"), "folder commands are Remote's own")
}
