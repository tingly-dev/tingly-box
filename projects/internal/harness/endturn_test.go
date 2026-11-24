package harness

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/coordtest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func assistantTexts(w *World) []string {
	var out []string
	for _, e := range w.Convo() {
		if e.Kind == db.EvAssistant {
			out = append(out, e.Content)
		}
	}
	return out
}

// E1: when a report needs no message to the user, the coordinator ends the
// turn with the no_update tool: nothing is posted, and the session was
// offered the tool.
func TestEndTurnNoUpdateToolStaysSilent(t *testing.T) {
	var offered []string
	w := New(t, Options{
		Files: projectsRepo,
		Actor: Diligent("done"),
		Brain: func(c *coordtest.Turn) error {
			switch {
			case c.CheckIn():
				offered = c.Tools()
				c.Call("no_update", map[string]string{"reason": "routine report"})
				// Text after no_update is not posted: the turn already ended.
				c.Reply("ignored")
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

	assert.Equal(t, []string{"Started."}, assistantTexts(w), "the report turn posted nothing")
	require.Len(t, w.Requests(), 2, "the user's turn and one turn for the report")
	assert.True(t, slices.Contains(offered, "no_update"), "the session is offered the tool")
}

// E2: a model that still answers with the old NO_UPDATE text stays silent.
func TestEndTurnLegacySentinelStaysSilent(t *testing.T) {
	w := New(t, Options{
		Files: projectsRepo,
		Actor: Diligent("done"),
		Brain: func(c *coordtest.Turn) error {
			switch {
			case c.CheckIn():
				c.Reply(NoUpdate)
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

	assert.Equal(t, []string{"Started."}, assistantTexts(w))
}

// E3: a user's message cannot be answered with no_update (the tool refuses
// it), and the old NO_UPDATE text is never posted as a reply.
func TestEndTurnNoUpdateOnUserMessageIsNotAReply(t *testing.T) {
	var refused string
	w := New(t, Options{
		Files: projectsRepo,
		Brain: func(c *coordtest.Turn) error {
			out, isErr := c.Call("no_update", map[string]string{})
			if isErr {
				refused = out
			}
			c.Reply(NoUpdate)
			return nil
		},
	})

	w.Say("What are you working on?")
	w.Quiet()

	assert.Contains(t, refused, "needs a reply")
	assert.Empty(t, assistantTexts(w))
}
