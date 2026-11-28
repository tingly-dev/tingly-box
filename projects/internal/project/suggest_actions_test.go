package project

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/coordtest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func bareCoordinator(t *testing.T, f *testFixture) *Coordinator {
	t.Helper()
	return NewCoordinator(f.store, Layout{Root: t.TempDir()}, NewHub(), f.threads)
}

func proposalCard(t *testing.T, f *testFixture, card ProposalCard) *db.ThreadEvent {
	t.Helper()
	p, err := f.store.GetProject(f.project.ID)
	require.NoError(t, err)
	e := &db.ThreadEvent{ThreadID: p.MainThreadID, Kind: db.EvSuggestions, Payload: jsonString(card)}
	require.NoError(t, f.store.AddThreadEvent(e))
	return e
}

func payloadOf(t *testing.T, store *db.Store, eventID string) suggestionsPayload {
	t.Helper()
	m, err := store.GetThreadEvent(eventID)
	require.NoError(t, err)
	var pl suggestionsPayload
	require.NoError(t, json.Unmarshal([]byte(m.Payload), &pl))
	return pl
}

func TestDismissSuggestionsPersists(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("ok", 1, 1); return nil })
	c := bareCoordinator(t, f)
	p, err := f.store.GetProject(f.project.ID)
	require.NoError(t, err)
	e := proposalCard(t, f, ProposalCard{Suggestions: []Suggestion{
		{Title: "A", Task: "do a"}, {Title: "B", Task: "do b"}, {Title: "C", Task: "do c"},
	}})

	got, err := c.DismissSuggestions(p.MainThreadID, e.ID, []int{1})
	require.NoError(t, err)
	pl := payloadOf(t, f.store, got.ID)
	assert.False(t, pl.Suggestions[0].Dismissed)
	assert.True(t, pl.Suggestions[1].Dismissed, "only the picked proposal is dismissed")
	assert.False(t, pl.Suggestions[2].Dismissed)

	// Empty indexes dismisses every remaining proposal.
	_, err = c.DismissSuggestions(p.MainThreadID, e.ID, nil)
	require.NoError(t, err)
	pl = payloadOf(t, f.store, e.ID)
	for i, s := range pl.Suggestions {
		assert.True(t, s.Dismissed, "proposal %d is dismissed", i)
	}

	// Dismissing again changes nothing: the card is already settled.
	again, err := c.DismissSuggestions(p.MainThreadID, e.ID, nil)
	require.NoError(t, err)
	assert.Equal(t, jsonString(pl), again.Payload)

	// A dismissed proposal is never started.
	_, err = c.StartSuggestions(context.Background(), p.MainThreadID, e.ID, nil, Source{})
	require.NoError(t, err)
	assert.Empty(t, threadsOf(t, f), "dismissed proposals do not start threads")
}

func TestPerformCardActionDismiss(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("ok", 1, 1); return nil })
	c := bareCoordinator(t, f)
	p, err := f.store.GetProject(f.project.ID)
	require.NoError(t, err)
	e := proposalCard(t, f, ProposalCard{Suggestions: []Suggestion{{Title: "A", Task: "a"}, {Title: "B", Task: "b"}}})

	m, err := c.PerformCardAction(context.Background(), p.MainThreadID, e.ID, ActionDismiss, []int{0}, Source{})
	require.NoError(t, err)
	pl := payloadOf(t, f.store, m.ID)
	assert.True(t, pl.Suggestions[0].Dismissed)
	assert.False(t, pl.Suggestions[1].Dismissed)
}

func TestPerformCardActionCustomRoundTrip(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("ok", 1, 1); return nil })
	c := bareCoordinator(t, f)
	p, err := f.store.GetProject(f.project.ID)
	require.NoError(t, err)

	var gotIndexes []int
	c.RegisterCardAction("remember", func(ctx context.Context, p *db.Project, eventID string, indexes []int, src Source) error {
		gotIndexes = append([]int(nil), indexes...)
		return nil
	})
	e := proposalCard(t, f, ProposalCard{
		Suggestions: []Suggestion{{Title: "A", Task: "a"}, {Title: "B", Task: "b"}},
		Actions:     []CardAction{{ID: "remember", Label: "Remember both", Style: "default", Scope: "card"}},
	})

	m, err := c.PerformCardAction(context.Background(), p.MainThreadID, e.ID, "remember", []int{0, 1}, Source{})
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, []int{0, 1}, gotIndexes, "a service-registered action gets the proposal indexes")

	_, err = c.PerformCardAction(context.Background(), p.MainThreadID, e.ID, "nope", nil, Source{})
	require.ErrorContains(t, err, "does not offer", "an action the card does not name is refused")

	c.RegisterCardAction("hidden", func(context.Context, *db.Project, string, []int, Source) error { return nil })
	_, err = c.PerformCardAction(context.Background(), p.MainThreadID, e.ID, "hidden", nil, Source{})
	require.ErrorContains(t, err, "does not offer", "a registered action still has to be declared on the card")
}

func TestProposalCardWithoutActionsUsesDefaults(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("ok", 1, 1); return nil })
	c := bareCoordinator(t, f)
	p, err := f.store.GetProject(f.project.ID)
	require.NoError(t, err)

	m, err := c.ProposeCard(context.Background(), p, ProposalCard{Suggestions: []Suggestion{{Title: "A", Task: "do a"}}})
	require.NoError(t, err)
	assert.NotContains(t, m.Payload, `"actions"`, "an omitted action set stays out of the payload and the frontend uses its defaults")
	assert.Len(t, DefaultCardActions(), 4)
}

func TestProposeCardSupersedesOlderCard(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("ok", 1, 1); return nil })
	c := bareCoordinator(t, f)
	p, err := f.store.GetProject(f.project.ID)
	require.NoError(t, err)

	old, err := c.ProposeCard(context.Background(), p, ProposalCard{Suggestions: []Suggestion{{Title: "Old", Task: "old"}}})
	require.NoError(t, err)
	fresh, err := c.ProposeCard(context.Background(), p, ProposalCard{Suggestions: []Suggestion{{Title: "New", Task: "new"}}})
	require.NoError(t, err)

	assert.True(t, payloadOf(t, f.store, old.ID).Suggestions[0].Superseded)
	assert.False(t, payloadOf(t, f.store, fresh.ID).Suggestions[0].Superseded)
}

func TestNewInstructionSupersedesProposals(t *testing.T) {
	f, svc := newServiceFixture(t, testHost{}, func(turn *enginetest.Turn) error {
		turn.Say("ok", 1, 1)
		return nil
	}, func(c *coordtest.Turn) error {
		if c.CheckIn() {
			c.Reply(noUpdate)
			return nil
		}
		c.Reply(noUpdate)
		return nil
	})
	coord := svc.Coordinator

	card, err := coord.ProposeCard(context.Background(), f.project, ProposalCard{Suggestions: []Suggestion{
		{Title: "A", Task: "do a"}, {Title: "B", Task: "do b"},
	}})
	require.NoError(t, err)

	_, err = coord.Post(f.project.ID, "Actually, change the plan.", nil, Source{})
	require.NoError(t, err)

	pl := payloadOf(t, f.store, card.ID)
	for i, s := range pl.Suggestions {
		assert.True(t, s.Superseded, "proposal %d is expired by the new instruction", i)
	}

	// Superseded proposals stay viewable and startable.
	updated, err := coord.StartSuggestions(context.Background(), f.project.MainThreadID, card.ID, []int{0}, Source{})
	require.NoError(t, err)
	pl = payloadOf(t, f.store, updated.ID)
	assert.NotEmpty(t, pl.Suggestions[0].ThreadID, "an expired proposal can still be started")
	assert.True(t, pl.Suggestions[1].Superseded, "the untouched proposal stays expired")
}
