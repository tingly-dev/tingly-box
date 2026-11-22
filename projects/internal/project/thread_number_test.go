package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// The user names threads by the number the thread list shows (#3); the
// coordinator sees that number and its tools take it beside the id prefix.
func TestCoordinatorNamesThreadsByNumber(t *testing.T) {
	f := newFixture(t, nil)
	c := NewCoordinator(f.store, f.threads.layout, f.threads.hub, f.threads)
	t.Cleanup(func() { settleCoordinator(c) })
	p := f.project
	var made []*db.Thread
	for _, title := range []string{"Fix login", "Tune the pool", "Add tracing"} {
		th := &db.Thread{ProjectID: p.ID, Title: title, Status: db.ThreadIdle, Origin: "user"}
		require.NoError(t, c.store.CreateThread(th))
		made = append(made, th)
	}
	second := made[1]
	require.Positive(t, second.Seq)
	num := "#" + itoa(second.Seq)

	for _, ref := range []string{num, " " + num + " ", num + " " + short(second.ID), num + " Tune the pool", second.ID[:8], second.ID} {
		got, err := c.findThread(p.ID, ref)
		require.NoError(t, err, ref)
		assert.Equal(t, second.ID, got.ID, ref)
	}
	for _, ref := range []string{"#0", "#999", "#x", "#"} {
		_, err := c.findThread(p.ID, ref)
		assert.Error(t, err, ref)
	}

	assert.Equal(t, num+" "+short(second.ID), threadName(second))
	assert.Equal(t, short(second.ID), threadName(&db.Thread{ID: second.ID}))

	list, err := c.listThreads(p, "", "pool", 5)
	require.NoError(t, err)
	assert.Contains(t, list, "- "+num+" "+short(second.ID)+` "Tune the pool"`)
}
