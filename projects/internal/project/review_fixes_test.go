package project

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// A card whose second suggestion cannot start must not forget the first one
// that did, or the next click starts it again.
func TestStartSuggestionsKeepsWhatItStartedWhenALaterOneFails(t *testing.T) {
	f, svc := newServiceFixture(t, testHost{}, idleScript, nil)
	card := svc.Coordinator.addEvent(f.project.ID, &db.ThreadEvent{Kind: db.EvSuggestions, Payload: jsonString(suggestionsPayload{Suggestions: []Suggestion{
		{Title: "Good", Task: "a", Repo: "none"},
		{Title: "Bad", Task: "b", Base: "no-such-branch"},
	}})})

	_, err := svc.Coordinator.StartSuggestions(context.Background(), f.project.MainThreadID, card.ID, nil)
	require.Error(t, err)
	require.Len(t, threadsOf(t, f), 1)

	saved, err := f.store.GetThreadEvent(card.ID)
	require.NoError(t, err)
	var pl suggestionsPayload
	require.NoError(t, json.Unmarshal([]byte(saved.Payload), &pl))
	assert.NotEmpty(t, pl.Suggestions[0].ThreadID, "the started suggestion is recorded on the card")
	assert.Empty(t, pl.Suggestions[1].ThreadID)

	_, err = svc.Coordinator.StartSuggestions(context.Background(), f.project.MainThreadID, card.ID, nil)
	require.Error(t, err, "the bad one still fails")
	assert.Len(t, threadsOf(t, f), 1, "and the good one is not started a second time")
}
