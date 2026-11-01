package harness

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/llm"
)

// seedConversation puts n older exchanges into the main thread, one of them
// carrying the fact the user will ask about later.
func seedConversation(w *World, n int) {
	for i := 0; i < n; i++ {
		text := fmt.Sprintf("chatter %d", i)
		if i == 3 {
			text = "For staging, rotate the API key every Monday."
		}
		for _, kind := range []string{db.EvUser, db.EvAssistant} {
			require.NoError(w.T, w.Store.AddThreadEvent(&db.ThreadEvent{ThreadID: w.Project.MainThreadID, Kind: kind, Content: text}))
		}
	}
}

// H1: when the conversation is longer than the window the coordinator sees,
// it is told so and can read the part it cannot see, instead of asking the
// user to repeat it.
func TestCoordinatorReadsConversationBeyondItsWindow(t *testing.T) {
	var told bool
	var read string
	w := New(t, Options{
		Files: projectsRepo,
		Brain: func(c *BrainCtx) llm.Result {
			switch {
			case c.AfterTools():
				read = c.Req.Messages[len(c.Req.Messages)-1].ToolResults[0].Content
				return llm.Result{Text: "Every Monday."}
			case strings.Contains(c.Last(), "how often"):
				told = strings.Contains(c.Req.System, "read_conversation")
				return llm.Result{ToolCalls: []llm.ToolCall{Call("r", "read_conversation", map[string]any{"contains": "rotate"})}}
			default:
				return llm.Result{Text: "ok"}
			}
		},
	})
	seedConversation(w, 40)

	w.Say("how often do we rotate the staging key?")
	w.Eventually("answered", func() bool { return strings.Contains(strings.Join(assistantTexts(w), "\n"), "Every Monday.") })
	w.Quiet()

	assert.True(t, told, "the coordinator is told the window is cut")
	assert.Contains(t, read, "rotate the API key every Monday")
	assert.Contains(t, read, `source="history"`)
	assert.Contains(t, read, `trust="relay"`, "old words come back as quoted history, not as the user speaking now")
}

// H2: a short conversation is shown whole and carries no cut-off note.
func TestShortConversationHasNoCutOffNote(t *testing.T) {
	var system string
	w := New(t, Options{
		Files: projectsRepo,
		Brain: func(c *BrainCtx) llm.Result {
			system = c.Req.System
			return llm.Result{Text: "ok"}
		},
	})
	seedConversation(w, 5)
	w.Say("hello")
	w.Quiet()
	assert.NotContains(t, system, "earlier messages exist")
}

// H3: a long message shows cut in a search, with the way to read it whole,
// and the coordinator reads it whole by id.
func TestCoordinatorReadsACutMessageWhole(t *testing.T) {
	var cut, whole string
	w := New(t, Options{
		Files: projectsRepo,
		Brain: func(c *BrainCtx) llm.Result {
			last := c.Req.Messages[len(c.Req.Messages)-1]
			switch {
			case c.AfterTools() && cut == "":
				cut = last.ToolResults[0].Content
				id := regexp.MustCompile(`read_conversation id=([0-9a-f]{8})`).FindStringSubmatch(cut)
				require.NotNil(t, id, "the cut entry says how to read it whole")
				return llm.Result{ToolCalls: []llm.ToolCall{Call("m", "read_conversation", map[string]any{"id": id[1]})}}
			case c.AfterTools():
				whole = last.ToolResults[0].Content
				return llm.Result{Text: "Read it."}
			case strings.Contains(c.Last(), "the brief"):
				return llm.Result{ToolCalls: []llm.ToolCall{Call("r", "read_conversation", map[string]any{"contains": "brief-start"})}}
			default:
				return llm.Result{Text: "ok"}
			}
		},
	})
	require.NoError(t, w.Store.AddThreadEvent(&db.ThreadEvent{ThreadID: w.Project.MainThreadID, Kind: db.EvUser,
		Content: "brief-start " + strings.Repeat("filler ", 400) + "the ending matters"}))
	seedConversation(w, 40)

	w.Say("what was in the brief?")
	w.Eventually("read", func() bool { return strings.Contains(strings.Join(assistantTexts(w), "\n"), "Read it.") })
	w.Quiet()

	assert.NotContains(t, cut, "the ending matters", "the search shows a cut-short message")
	assert.Contains(t, whole, "the ending matters", "reading it by id gives it whole")
}
