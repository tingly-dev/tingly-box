package harness

import (
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/coordtest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// A thread that wants another thread (design: thread-coordinator-channel.md).
// These pin the baseline that the notify_coordinator change starts from. The
// brain is scripted, so they check the system around the model (what the
// thread can reach, how its words arrive, what a card does), not whether a
// real model would choose to propose.

const (
	spawnAsk = "Splitting this out would help: a separate thread should handle the rename tool."
	renameTh = "Add a thread rename tool"
)

// T1: the tools of a thread's own session. None of them reaches the
// coordinator, which is why the only way to ask is the report. Adding
// notify_coordinator changes this list on purpose.
func TestThreadSessionHasNoToolToReachTheCoordinator(t *testing.T) {
	var tools []string
	var mu sync.Mutex
	w := New(t, Options{
		Files: projectsRepo,
		Brain: quietStart(endTool),
		Actor: func(a *ActorCtx) error {
			session, err := coordtest.ToolSession(a.Turn.Ctx, a.Turn.Opts.MCPConfigPath)
			require.NoError(t, err)
			defer session.Close()
			res, err := session.ListTools(a.Turn.Ctx, nil)
			require.NoError(t, err)
			mu.Lock()
			for _, tool := range res.Tools {
				tools = append(tools, tool.Name)
			}
			mu.Unlock()
			a.Say("Done.")
			return nil
		},
	})
	w.Say("Add an end-of-turn tool.")
	w.ThreadIs(endTool, db.ThreadIdle)
	w.Quiet()

	mu.Lock()
	defer mu.Unlock()
	slices.Sort(tools)
	assert.Equal(t, []string{"comment", "create_issue", "create_pr", "merge", "send_file", "update", "view"}, tools,
		"a thread's tools; no start_thread, propose_threads, message_thread or anything that reaches the coordinator")
}

// T2: a thread asks for another thread in its report. It arrives labelled as
// the thread's words; the coordinator offers it as a card instead of starting
// it; nothing runs until the user starts the card, and the thread that
// starts is marked as a suggestion's.
func TestJourneyThreadRequestForAThreadBecomesACard(t *testing.T) {
	var proposal string
	w := New(t, Options{
		Files: projectsRepo,
		Actor: Reply("Done with the end-of-turn tool.\n\n" + spawnAsk),
		Brain: func(c *coordtest.Turn) error {
			if c.CheckIn() {
				proposal, _ = c.Call("propose_threads", map[string]any{"threads": []map[string]string{
					{"title": renameTh, "task": "Add a rename tool for threads, with tests."}}})
				c.Reply("The thread suggests a rename tool; start it from the card if you want it.")
				return nil
			}
			startThread(c, endTool, "Do it.")
			c.Reply("Started.")
			return nil
		},
	})

	w.Say("Add an end-of-turn tool.")
	w.ThreadIs(endTool, db.ThreadIdle)
	w.Quiet()

	req, ok := reportRequest(w)
	require.True(t, ok, "the report reached the coordinator")
	require.Contains(t, req.Prompt, spawnAsk)
	assert.Less(t, strings.Index(req.Prompt, `trust="relay"`), strings.Index(req.Prompt, spawnAsk), "the label comes before the thread's words")
	assert.Contains(t, proposal, "proposed 1 threads")

	assert.Len(t, w.ThreadList(), 1, "the proposal started nothing")
	var card *db.ThreadEvent
	for _, e := range w.Convo() {
		if e.Kind == db.EvSuggestions {
			e := e
			card = &e
		}
	}
	require.NotNil(t, card, "the user sees a card")

	_, err := w.Coord.StartSuggestions(t.Context(), w.Project.MainThreadID, card.ID, nil)
	require.NoError(t, err)
	started := w.ThreadIs(renameTh, db.ThreadIdle)
	assert.Equal(t, "suggestion", started.Origin, "the user's click started it, not the coordinator")
}

// T3: the coordinator's tool is not a way around the card. start_thread from a
// report's words is possible for a model (nothing refuses it), so this pins the
// one thing that does hold: the thread cannot call it, and a thread's report
// is never an approval (see TestPersonaInjectionInAReportStaysData).
func TestThreadCannotStartAThreadItself(t *testing.T) {
	var out string
	var isErr bool
	w := New(t, Options{
		Files: projectsRepo,
		Brain: quietStart(endTool),
		Actor: func(a *ActorCtx) error {
			out, isErr = callTool(a, "start_thread", map[string]any{"title": renameTh, "task": "x"})
			a.Say("Tried.")
			return nil
		},
	})
	w.Say("Add an end-of-turn tool.")
	w.ThreadIs(endTool, db.ThreadIdle)
	w.Quiet()

	assert.True(t, isErr, out)
	assert.Len(t, w.ThreadList(), 1, "no thread was started by the thread")
}
