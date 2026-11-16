package harness

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/coordtest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/internal/project"
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

func askMerge(c *coordtest.Turn) {
	c.Call("request_decision", map[string]any{
		"question": "Merge the harness now?",
		"options": []map[string]string{
			{"label": "Merge now", "consequence": "main gets the harness."},
			{"label": "Wait for review", "consequence": "nothing changes yet."},
		},
		"recommended": 1,
		"reason":      "Merging cannot be undone.",
	})
	c.Reply("Asked.")
}

// D1: a choice that cannot be undone is put to the user on a card; the pick
// reaches the coordinator as the user's own message and cannot be changed.
func TestDecisionCardIsAskedAndAnswered(t *testing.T) {
	w := New(t, Options{
		Files: projectsRepo,
		Brain: func(c *coordtest.Turn) error {
			if strings.Contains(c.Input(), "Wait for review") {
				c.Reply("Holding the merge until review.")
				return nil
			}
			askMerge(c)
			return nil
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

// D2: a malformed card is refused with a reason and nothing is posted. Too
// few options fail the tool's input schema before the card is built; a
// recommendation outside the options fails the card's own check.
func TestDecisionCardRejectsMalformedAsks(t *testing.T) {
	var refused []string
	w := New(t, Options{
		Files: projectsRepo,
		Brain: func(c *coordtest.Turn) error {
			var out string
			var isErr bool
			if strings.Contains(c.Input(), "again") {
				out, isErr = c.Call("request_decision", map[string]any{
					"question": "Merge?", "options": []map[string]string{{"label": "Yes"}, {"label": "No"}}, "recommended": 5})
			} else {
				out, isErr = c.Call("request_decision", map[string]any{
					"question": "Merge?", "options": []map[string]string{{"label": "Yes"}}, "recommended": 0})
			}
			if isErr {
				refused = append(refused, out)
			}
			c.Reply("Never mind.")
			return nil
		},
	})

	w.Say("Merge?")
	w.Quiet()
	w.Say("Ask again.")
	w.Quiet()

	require.Len(t, refused, 2)
	assert.Contains(t, refused[0], "invalid input")
	assert.Contains(t, refused[1], "recommended must be")
	assert.Nil(t, decisionEvent(w))
}

// D3: pressing a card twice at once (a double click, two tabs) records one
// pick and posts one message.
func TestDecisionCardIsAnsweredOnce(t *testing.T) {
	w := New(t, Options{
		Files: projectsRepo,
		Brain: func(c *coordtest.Turn) error {
			if strings.Contains(c.Input(), "Wait for review") || strings.Contains(c.Input(), "Merge now") {
				c.Reply("Noted.")
				return nil
			}
			askMerge(c)
			return nil
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
