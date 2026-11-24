package harness

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/projects/internal/coordtest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// R1: when a thread's goal changes, the coordinator renames its card; the
// work, branch and history stay, and the next turn sees the new title.
func TestRenameThreadWhenTheGoalChanges(t *testing.T) {
	const newTitle = "Add end-of-turn and rename tools"
	w := New(t, Options{
		Files: projectsRepo,
		Actor: Diligent("done"),
		Brain: func(c *coordtest.Turn) error {
			switch {
			case c.CheckIn():
				c.Call("no_update", map[string]string{})
			case strings.Contains(c.Input(), "rename tool too"):
				c.Call("rename_thread", map[string]string{"thread_id": c.ThreadRef(endTool), "title": newTitle})
				c.Reply("Renamed.")
			default:
				startThread(c, endTool, "Add the no_update tool.")
				c.Reply("Started.")
			}
			return nil
		},
	})

	w.Say("Add an end-of-turn tool.")
	before := w.ThreadIs(endTool, db.ThreadReady)
	w.Quiet()

	w.Say("Make it the rename tool too.")
	w.Eventually("renamed", func() bool { return len(w.ThreadList()) == 1 && w.ThreadList()[0].Title == newTitle })
	w.Quiet()

	after := w.ThreadList()[0]
	assert.Equal(t, before.ID, after.ID)
	assert.Equal(t, before.Branch, after.Branch, "the branch keeps its name")
	assert.Equal(t, db.ThreadReady, after.Status)
	assert.Len(t, w.Prompts(), 1, "renaming does not start a turn")
}

// R2: an empty title is refused and the thread keeps its name.
func TestRenameThreadRejectsAnEmptyTitle(t *testing.T) {
	var refused string
	w := New(t, Options{
		Files: projectsRepo,
		Actor: Diligent("done"),
		Brain: func(c *coordtest.Turn) error {
			switch {
			case c.CheckIn():
				c.Call("no_update", map[string]string{})
			case strings.Contains(c.Input(), "rename"):
				if out, isErr := c.Call("rename_thread", map[string]string{"thread_id": c.ThreadRef(endTool), "title": "  "}); isErr {
					refused = out
				}
				c.Reply("Could not rename.")
			default:
				startThread(c, endTool, "Add the no_update tool.")
				c.Reply("Started.")
			}
			return nil
		},
	})

	w.Say("Add an end-of-turn tool.")
	w.ThreadIs(endTool, db.ThreadReady)
	w.Quiet()
	w.Say("Please rename it.")
	w.Quiet()

	assert.NotEmpty(t, refused)
	assert.Equal(t, endTool, w.ThreadList()[0].Title)
}
