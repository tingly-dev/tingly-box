package harness

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/internal/project"
	"github.com/tingly-dev/tingly-box/projects/llm"
)

func decisionEvent(w *World) *db.ThreadEvent {
	for _, e := range w.Convo() {
		if e.Kind == db.EvDecision {
			e := e
			return &e
		}
	}
	return nil
}

func askMerge() llm.Result {
	return llm.Result{ToolCalls: []llm.ToolCall{Call("d1", "request_decision", map[string]any{
		"question": "Merge the harness now?",
		"options": []map[string]string{
			{"label": "Merge now", "consequence": "main gets the harness."},
			{"label": "Wait for review", "consequence": "nothing changes yet."},
		},
		"recommended": 1,
		"reason":      "Merging cannot be undone.",
	})}}
}

// D1: a choice that cannot be undone is put to the user on a card; the pick
// reaches the coordinator as the user's own message and cannot be changed.
func TestDecisionCardIsAskedAndAnswered(t *testing.T) {
	w := New(t, Options{
		Files: projectsRepo,
		Brain: func(c *BrainCtx) llm.Result {
			switch {
			case c.AfterTools():
				return llm.Result{Text: "Asked."}
			case strings.Contains(c.Last(), "Wait for review"):
				return llm.Result{Text: "Holding the merge until review."}
			default:
				return askMerge()
			}
		},
	})

	w.Say("Should we merge the harness?")
	w.Eventually("decision card", func() bool { return decisionEvent(w) != nil })
	w.Quiet()

	card := decisionEvent(w)
	var d project.Decision
	require.NoError(t, json.Unmarshal([]byte(card.Payload), &d))
	assert.Equal(t, 1, d.Recommended)
	assert.Nil(t, d.Chosen)
	assert.Contains(t, card.Content, "2. Wait for review — nothing changes yet. (recommended)", "the card in words, for chats without cards")

	_, err := w.Coord.Choose(context.Background(), w.Project.MainThreadID, card.ID, 1, project.Source{})
	require.NoError(t, err)
	w.Eventually("coordinator answered the pick", func() bool {
		return strings.Contains(strings.Join(assistantTexts(w), "\n"), "Holding the merge until review.")
	})
	w.Quiet()

	var after project.Decision
	require.NoError(t, json.Unmarshal([]byte(decisionEvent(w).Payload), &after))
	require.NotNil(t, after.Chosen)
	assert.Equal(t, 1, *after.Chosen)

	_, err = w.Coord.Choose(context.Background(), w.Project.MainThreadID, card.ID, 0, project.Source{})
	assert.ErrorIs(t, err, project.ErrDecided, "a second pick is refused")
}

// D2: a malformed card is refused with a reason and nothing is posted.
func TestDecisionCardRejectsMalformedAsks(t *testing.T) {
	var refused []string
	w := New(t, Options{
		Files: projectsRepo,
		Brain: func(c *BrainCtx) llm.Result {
			switch {
			case c.AfterTools():
				refused = append(refused, c.Req.Messages[len(c.Req.Messages)-1].ToolResults[0].Content)
				return llm.Result{Text: "Never mind."}
			case len(refused) == 0 && !strings.Contains(c.Last(), "again"):
				return llm.Result{ToolCalls: []llm.ToolCall{Call("d", "request_decision", map[string]any{
					"question": "Merge?", "options": []map[string]string{{"label": "Yes"}}, "recommended": 0})}}
			default:
				return llm.Result{ToolCalls: []llm.ToolCall{Call("d", "request_decision", map[string]any{
					"question": "Merge?", "options": []map[string]string{{"label": "Yes"}, {"label": "No"}}, "recommended": 5})}}
			}
		},
	})

	w.Say("Merge?")
	w.Quiet()
	w.Say("Ask again.")
	w.Quiet()

	require.Len(t, refused, 2)
	assert.Contains(t, refused[0], "two to four options")
	assert.Contains(t, refused[1], "recommended must be")
	assert.Nil(t, decisionEvent(w))
}

// D3: pressing a card twice at once (a double click, two tabs) records one
// pick and posts one message.
func TestDecisionCardIsAnsweredOnce(t *testing.T) {
	w := New(t, Options{
		Files: projectsRepo,
		Brain: func(c *BrainCtx) llm.Result {
			switch {
			case c.AfterTools():
				return llm.Result{Text: "Asked."}
			case strings.Contains(c.Last(), "Wait for review") || strings.Contains(c.Last(), "Merge now"):
				return llm.Result{Text: "Noted."}
			default:
				return askMerge()
			}
		},
	})
	w.Say("Should we merge the harness?")
	w.Eventually("decision card", func() bool { return decisionEvent(w) != nil })
	w.Quiet()
	card := decisionEvent(w)

	var wg sync.WaitGroup
	errs := make([]error, 6)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = w.Coord.Choose(context.Background(), w.Project.MainThreadID, card.ID, i%2, project.Source{})
		}(i)
	}
	wg.Wait()
	w.Quiet()

	won := 0
	for _, err := range errs {
		if err == nil {
			won++
		} else {
			assert.ErrorIs(t, err, project.ErrDecided)
		}
	}
	assert.Equal(t, 1, won, "exactly one pick wins")
	picks := 0
	for _, e := range w.Convo() {
		if e.Kind == db.EvUser && (e.Content == "Merge now" || e.Content == "Wait for review") {
			picks++
		}
	}
	assert.Equal(t, 1, picks, "one message reaches the coordinator")
}
