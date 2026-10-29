package harness

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/llm"
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
// turn with the no_update tool: nothing is posted and no second model round
// is spent.
func TestEndTurnNoUpdateToolStaysSilent(t *testing.T) {
	w := New(t, Options{
		Files: projectsRepo,
		Actor: Diligent("done"),
		Brain: func(c *BrainCtx) llm.Result {
			switch {
			case c.AfterTools():
				return llm.Result{Text: "Started."}
			case c.CheckIn():
				return llm.Result{ToolCalls: []llm.ToolCall{Call("n1", "no_update", map[string]string{"reason": "routine report"})}}
			default:
				return startThread(endTool, "Add the no_update tool.")
			}
		},
	})

	w.Say("Add an end-of-turn tool.")
	w.ThreadIs(endTool, db.ThreadReady)
	// The report reaches the coordinator a moment after the thread is ready;
	// on a slow runner Quiet alone can return before the report turn begins.
	w.Eventually("the report turn ran", func() bool { return len(w.Requests()) >= 3 })
	w.Quiet()

	assert.Equal(t, []string{"Started."}, assistantTexts(w), "the report turn posted nothing")
	reqs := w.Requests()
	require.Len(t, reqs, 3, "start, start's tool round, and exactly one request for the report turn")
	assert.Contains(t, toolNames(reqs[2]), "no_update", "the model is offered the tool")
}

// E2: a model that still answers with the old NO_UPDATE text stays silent.
func TestEndTurnLegacySentinelStaysSilent(t *testing.T) {
	w := New(t, Options{
		Files: projectsRepo,
		Actor: Diligent("done"),
		Brain: func(c *BrainCtx) llm.Result {
			switch {
			case c.AfterTools():
				return llm.Result{Text: "Started."}
			case c.CheckIn():
				return llm.Result{Text: NoUpdate}
			default:
				return startThread(endTool, "Add the no_update tool.")
			}
		},
	})

	w.Say("Add an end-of-turn tool.")
	w.ThreadIs(endTool, db.ThreadReady)
	w.Quiet()

	assert.Equal(t, []string{"Started."}, assistantTexts(w))
}

// E3: a turn that answers the user with nothing is asked once to reply, so
// a message never goes unanswered.
func TestEndTurnEmptyReplyToUserIsAskedAgain(t *testing.T) {
	w := New(t, Options{
		Files: projectsRepo,
		Brain: func(c *BrainCtx) llm.Result {
			if strings.Contains(c.Last(), "ended your turn without replying") {
				return llm.Result{Text: "Sorry, here is the answer."}
			}
			return llm.Result{}
		},
	})

	w.Say("What are you working on?")
	w.Quiet()

	assert.Equal(t, []string{"Sorry, here is the answer."}, assistantTexts(w))
	assert.Len(t, w.Requests(), 2)
}

// E4: asked twice, an empty answer ends the turn: the loop is bounded.
func TestEndTurnEmptyReplyIsAskedOnlyOnce(t *testing.T) {
	w := New(t, Options{
		Files: projectsRepo,
		Brain: func(c *BrainCtx) llm.Result { return llm.Result{} },
	})

	w.Say("Hello?")
	w.Quiet()

	assert.Empty(t, assistantTexts(w))
	assert.Len(t, w.Requests(), 2)
}

func toolNames(r llm.Request) []string {
	var out []string
	for _, tl := range r.Tools {
		out = append(out, tl.Name)
	}
	return out
}
