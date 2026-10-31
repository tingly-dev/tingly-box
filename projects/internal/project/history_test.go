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

	out, err := c.readConversation(p, readArgs{Limit: 5})
	require.NoError(t, err)
	// The window holds m050..m089; the five before it come back, oldest first.
	assert.Contains(t, out, "m049")
	assert.Contains(t, out, "m045")
	assert.NotContains(t, out, "m050")
	assert.NotContains(t, out, "m044")

	first := strings.Fields(strings.SplitN(out, "[", 2)[1])[0]
	next, err := c.readConversation(p, readArgs{Before: first, Limit: 5})
	require.NoError(t, err)
	assert.Contains(t, next, "m044")
	assert.NotContains(t, next, "m045")

	found, err := c.readConversation(p, readArgs{Contains: "m007", Limit: 5})
	require.NoError(t, err)
	assert.Contains(t, found, "m007")
	assert.NotContains(t, found, "m006")

	none, err := c.readConversation(p, readArgs{Contains: "nothing like this", Limit: 5})
	require.NoError(t, err)
	assert.Contains(t, none, "nothing earlier matches")

	_, err = c.readConversation(p, readArgs{Before: "zzzzzzzz", Limit: 5})
	assert.Error(t, err)
}

// The three caps: an entry is cut with a pointer to its whole, a result is
// kept under a total with the oldest entries hidden and said so, and an
// entry can be read whole by id.
func TestReadConversationIsCappedLikeGrep(t *testing.T) {
	f := newFixture(t, nil)
	c := NewCoordinator(f.store, f.threads.layout, f.threads.hub, f.threads, nil)
	t.Cleanup(func() { settleCoordinator(c) })
	p := f.project
	add := func(text string) *db.ThreadEvent {
		e := &db.ThreadEvent{ThreadID: p.MainThreadID, Kind: db.EvUser, Content: text}
		require.NoError(t, c.store.AddThreadEvent(e))
		return e
	}
	long := add("start " + strings.Repeat("x", 2000) + " the end")
	for i := 0; i < historyLimit; i++ {
		add(fmt.Sprintf("w%02d", i))
	}

	out, err := c.readConversation(p, readArgs{})
	require.NoError(t, err)
	assert.Contains(t, out, "cut, ", "a long entry is cut")
	assert.Contains(t, out, "read_conversation id="+long.ID[:8], "and says how to read it whole")
	assert.NotContains(t, out, "the end")

	whole, err := c.readConversation(p, readArgs{ID: long.ID[:8]})
	require.NoError(t, err)
	assert.Contains(t, whole, "the end")

	// Many mid-size entries overflow the total: the oldest are hidden.
	for i := 0; i < 30; i++ {
		add(fmt.Sprintf("big%02d %s", i, strings.Repeat("y", 500)))
	}
	for i := 0; i < historyLimit; i++ {
		add(fmt.Sprintf("z%02d", i))
	}
	capped, err := c.readConversation(p, readArgs{Limit: 40})
	require.NoError(t, err)
	assert.Contains(t, capped, "older entries hidden")
	assert.Contains(t, capped, "before=")
	assert.Less(t, len(capped), previewTotal+1000)
}

func TestListThreadsFiltersAndCaps(t *testing.T) {
	f := newFixture(t, nil)
	c := NewCoordinator(f.store, f.threads.layout, f.threads.hub, f.threads, nil)
	t.Cleanup(func() { settleCoordinator(c) })
	p := f.project
	for i := 0; i < 50; i++ {
		status := db.ThreadResolved
		if i%10 == 0 {
			status = db.ThreadWorking
		}
		require.NoError(t, c.store.CreateThread(&db.Thread{ProjectID: p.ID, Title: fmt.Sprintf("Fix thing %02d", i), Status: status, Origin: "coordinator"}))
	}

	all, err := c.listThreads(p, "", "", 0)
	require.NoError(t, err)
	assert.Equal(t, readItems, strings.Count(all, "\n- "), "default count")
	assert.Contains(t, all, "more match")

	working, err := c.listThreads(p, db.ThreadWorking, "", 40)
	require.NoError(t, err)
	assert.Equal(t, 5, strings.Count(working, "status=working"))
	assert.NotContains(t, working, "more match")

	one, err := c.listThreads(p, "", "thing 07", 5)
	require.NoError(t, err)
	assert.Contains(t, one, "Fix thing 07")

	none, err := c.listThreads(p, "", "absent", 5)
	require.NoError(t, err)
	assert.Equal(t, "no thread matches", none)
}
