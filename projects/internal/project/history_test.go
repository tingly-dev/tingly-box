package project

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestReadConversationPagesBackFromTheWindow(t *testing.T) {
	f := newFixture(t, nil)
	c := NewCoordinator(f.store, f.threads.layout, f.threads.hub, f.threads, nil)
	t.Cleanup(func() { settleCoordinator(c) })
	p := f.project
	for i := 0; i < historyLimit+50; i++ {
		require.NoError(t, c.store.AddThreadEvent(&db.ThreadEvent{ThreadID: p.MainThreadID, Kind: db.EvUser, Content: fmt.Sprintf("m%03d", i)}))
	}

	out, err := c.readConversation(p, "", "", 5)
	require.NoError(t, err)
	// The window holds m050..m089; the five before it come back, oldest first.
	assert.Contains(t, out, "m049")
	assert.Contains(t, out, "m045")
	assert.NotContains(t, out, "m050")
	assert.NotContains(t, out, "m044")

	first := strings.Fields(strings.SplitN(out, "[", 2)[1])[0]
	next, err := c.readConversation(p, first, "", 5)
	require.NoError(t, err)
	assert.Contains(t, next, "m044")
	assert.NotContains(t, next, "m045")

	found, err := c.readConversation(p, "", "m007", 5)
	require.NoError(t, err)
	assert.Contains(t, found, "m007")
	assert.NotContains(t, found, "m006")

	none, err := c.readConversation(p, "", "nothing like this", 5)
	require.NoError(t, err)
	assert.Contains(t, none, "nothing earlier matches")

	_, err = c.readConversation(p, "zzzzzzzz", "", 5)
	assert.Error(t, err)
}
