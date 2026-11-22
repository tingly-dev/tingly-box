package harness

import (
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/coordtest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/internal/project"
)

// A thread that wants another thread (design: thread-coordinator-channel.md).
// These pin the baseline that the notify_coordinator change starts from. The
// brain is scripted, so they check the system around the model (what the
// thread can reach, how its words arrive, what a card does), not whether a
// real model would choose to propose.

const (
	spawnAsk = "Splitting this out would help: a separate thread should handle the rename tool."
	renameTh = "Add a thread rename tool"
	// notifyEnvelope opens a notify event; the coordinator's own instructions
	// mention kind="notify" too, so the source goes with it.
	notifyEnvelope = `source="thread" kind="notify"`
)

// T1: the tools of a thread's own session. The only one that reaches the
// coordinator is notify_coordinator; a thread cannot start or message threads.
func TestThreadSessionReachesTheCoordinatorOnlyThroughNotify(t *testing.T) {
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
	assert.Equal(t, []string{"close", "comment", "create_issue", "create_pr", "merge", "notify_coordinator", "send_file", "update", "view"}, tools,
		"a thread's tools; no start_thread, propose_threads or message_thread")
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

	_, err := w.Coord.StartSuggestions(t.Context(), w.Project.MainThreadID, card.ID, nil, project.Source{})
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

// T4: a thread asks for another thread with notify_coordinator, mid-turn. The
// coordinator hears it as a relayed notify (never as the user) and offers it as
// a card at once; nothing starts until the user starts the card.
func TestJourneyThreadNotifyRequestBecomesACard(t *testing.T) {
	var proposals []string
	var notified string
	var notifyErr bool
	w := New(t, Options{
		Files: projectsRepo,
		Actor: func(a *ActorCtx) error {
			if strings.Contains(a.Prompt, "Do it.") { // the first thread only, not the one it asked for
				notified, notifyErr = callTool(a, "notify_coordinator", map[string]any{"message": spawnAsk})
			}
			a.Commit("change.txt", a.Prompt+"\n")
			a.Say("Done with the end-of-turn tool.")
			return nil
		},
		Brain: func(c *coordtest.Turn) error {
			switch {
			case strings.Contains(c.Prompt, notifyEnvelope) && len(proposals) == 0:
				out, _ := c.Call("propose_threads", map[string]any{"threads": []map[string]string{
					{"title": renameTh, "task": "Add a rename tool for threads, with tests."}}})
				proposals = append(proposals, out)
				c.Reply("The thread suggests a rename tool; start it from the card if you want it.")
			case c.CheckIn():
				c.Call("no_update", map[string]string{})
			case c.FromUser():
				startThread(c, endTool, "Do it.")
				c.Reply("Started.")
			default:
				c.Call("no_update", map[string]string{})
			}
			return nil
		},
	})

	w.Say("Add an end-of-turn tool.")
	w.ThreadIs(endTool, db.ThreadReady)
	w.Quiet()

	require.False(t, notifyErr, notified)
	require.Len(t, proposals, 1, "the coordinator offered it once")
	assert.Contains(t, proposals[0], "proposed 1 threads")
	var notifyReq *Request
	for _, r := range w.Requests() {
		if strings.Contains(r.Prompt, notifyEnvelope) {
			r := r
			notifyReq = &r
			break
		}
	}
	require.NotNil(t, notifyReq, "the notify reached the coordinator")
	assert.Contains(t, notifyReq.Prompt, spawnAsk)
	assert.Less(t, strings.Index(notifyReq.Prompt, `trust="relay"`), strings.Index(notifyReq.Prompt, spawnAsk), "labelled before the thread's words")

	assert.Len(t, w.ThreadList(), 1, "the proposal started nothing")
	var card *db.ThreadEvent
	for _, e := range w.Convo() {
		if e.Kind == db.EvSuggestions {
			e := e
			card = &e
		}
	}
	require.NotNil(t, card, "the user sees a card")
	_, err := w.Coord.StartSuggestions(t.Context(), w.Project.MainThreadID, card.ID, nil, project.Source{})
	require.NoError(t, err)
	assert.Equal(t, "suggestion", w.ThreadIs(renameTh, db.ThreadReady).Origin)
}

// T5: both kinds of session are launched with the native cross-session channel
// closed: no inbound turns, no ListAgents, SendMessage only through the
// approval prompt. The coordinator also has no SendMessage at all.
func TestSessionsAreLaunchedWithTheNativeChannelClosed(t *testing.T) {
	w := New(t, Options{Files: projectsRepo, Actor: Reply("Done."), Brain: quietStart(endTool)})
	w.Say("Add an end-of-turn tool.")
	w.ThreadIs(endTool, db.ThreadIdle)
	w.Quiet()

	var coordinators, threads int
	for _, l := range w.Agent.Launches() {
		b, err := os.ReadFile(l.SettingsPath)
		require.NoError(t, err, "a launch has a settings file")
		assert.Contains(t, string(b), `"crossSessionInbound":"refuse"`, l.SettingsPath)
		assert.Contains(t, string(b), `"ListAgents"`, "denied")
		if coordtest.IsCoordinator(l) {
			coordinators++
			assert.Contains(t, l.DisallowedTools, "SendMessage")
			assert.Contains(t, l.DisallowedTools, "ListAgents")
		} else {
			threads++
			assert.Contains(t, string(b), `"SendMessage"`, "asks")
		}
	}
	assert.Positive(t, coordinators)
	assert.Positive(t, threads)
}
