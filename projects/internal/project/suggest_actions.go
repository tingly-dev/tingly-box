package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// A proposal card is service-driven: the payload carries how it reads and
// which actions it offers, and the frontend draws them generically. The
// built-in actions (start, dismiss, and their all-proposals forms) are
// handled here; any other action id is handled by a handler a service
// registered with RegisterCardAction.

// The built-in card action ids.
const (
	ActionStart      = "start"
	ActionDismiss    = "dismiss"
	ActionStartAll   = "start_all"
	ActionDismissAll = "dismiss_all"
)

// CardAction is one button a proposal card offers. Scope is "item" (the
// default: the action acts on one proposal) or "card" (on the card's
// proposals together). Style is "primary", "default" or "danger"; an empty
// label is filled with the frontend's own words for a built-in action.
type CardAction struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
	Style string `json:"style,omitempty"`
	Scope string `json:"scope,omitempty"`
}

// DefaultCardActions is the action set a card gets when its payload names
// none: start or dismiss one proposal, and start or dismiss the rest.
func DefaultCardActions() []CardAction {
	return []CardAction{
		{ID: ActionStart, Style: "primary", Scope: "item"},
		{ID: ActionDismiss, Style: "default", Scope: "item"},
		{ID: ActionStartAll, Style: "primary", Scope: "card"},
		{ID: ActionDismissAll, Style: "default", Scope: "card"},
	}
}

// CardActionHandler runs an action a service registered for its proposal
// cards. It is given the project, the card's event id and the proposal
// indexes the action applies to (empty for the whole card); it should update
// the event itself when the action changes it.
type CardActionHandler func(ctx context.Context, p *db.Project, eventID string, indexes []int, src Source) error

// RegisterCardAction makes id an action services may put on a proposal card.
// It replaces an earlier handler for the same idea. A nil handler removes it.
func (c *Coordinator) RegisterCardAction(id string, h CardActionHandler) {
	c.actionsMu.Lock()
	defer c.actionsMu.Unlock()
	if c.actions == nil {
		c.actions = map[string]CardActionHandler{}
	}
	if h == nil {
		delete(c.actions, id)
		return
	}
	c.actions[id] = h
}

func (c *Coordinator) cardActionHandler(id string) CardActionHandler {
	c.actionsMu.Lock()
	defer c.actionsMu.Unlock()
	return c.actions[id]
}

// ProposeCard posts a proposal card to a project's conversation, offering
// its threads to the user. It supersedes the older cards still waiting, so
// only the newest plan offers its start buttons, and tells the chats like
// the coordinator's own propose_threads does. Services drive their own
// proposal cards through it.
func (c *Coordinator) ProposeCard(ctx context.Context, p *db.Project, card ProposalCard) (*db.ThreadEvent, error) {
	if len(card.Suggestions) == 0 {
		return nil, errors.New("a proposal card needs at least one proposal")
	}
	if err := checkProposals(ctx, p, card.Suggestions); err != nil {
		return nil, err
	}
	c.SupersedeSuggestions(p.MainThreadID, "")
	m := c.addEvent(p.ID, &db.ThreadEvent{Kind: db.EvSuggestions, Payload: jsonString(card)})
	c.final(m)
	return m, nil
}

// DismissSuggestions declines proposals of a suggestions card of a main
// thread, so they never start and stay dismissed after a reload. indexes
// selects which; empty dismisses every proposal not started yet. It is the
// user's own answer, so the card keeps the record instead of disappearing.
func (c *Coordinator) DismissSuggestions(mainThreadID, eventID string, indexes []int) (*db.ThreadEvent, error) {
	c.cards.Lock()
	defer c.cards.Unlock()
	main, m, pl, err := c.cardEvent(mainThreadID, eventID)
	if err != nil {
		return nil, err
	}
	want := indexSet(indexes)
	changed := false
	for i := range pl.Suggestions {
		s := &pl.Suggestions[i]
		if s.ThreadID != "" || s.Dismissed || (len(want) > 0 && !want[i]) {
			continue
		}
		s.Dismissed = true
		s.Superseded = false
		changed = true
	}
	if !changed {
		return m, nil
	}
	m.Payload = jsonString(pl)
	if err := c.store.UpdateThreadEvent(m); err != nil {
		return nil, err
	}
	c.publishEvent(main.ProjectID, m)
	return m, nil
}

// PerformCardAction runs an action a proposal card offers. The built-in
// actions start or dismiss proposals (empty indexes means all of them); any
// other id must be declared on the card and registered by a service.
func (c *Coordinator) PerformCardAction(ctx context.Context, mainThreadID, eventID, action string, indexes []int, src Source) (*db.ThreadEvent, error) {
	switch action {
	case ActionStart, ActionStartAll:
		return c.StartSuggestions(ctx, mainThreadID, eventID, indexes, src)
	case ActionDismiss, ActionDismissAll:
		return c.DismissSuggestions(mainThreadID, eventID, indexes)
	}
	main, m, pl, err := c.cardEvent(mainThreadID, eventID)
	if err != nil {
		return nil, err
	}
	if !cardDeclares(pl, action) {
		return nil, fmt.Errorf("the card does not offer %q", action)
	}
	h := c.cardActionHandler(action)
	if h == nil {
		return nil, fmt.Errorf("no service handles the card action %q", action)
	}
	p, err := c.store.GetProject(main.ProjectID)
	if err != nil {
		return nil, err
	}
	if err := h(ctx, p, m.ID, indexes, src); err != nil {
		return nil, err
	}
	return c.store.GetThreadEvent(eventID)
}

// SupersedeSuggestions marks every proposal still waiting on the named
// main thread as superseded: a new instruction has made the older plan
// stale. Started and dismissed proposals are left as they are, and the card
// a new action is about is passed as exceptEventID. Superseded proposals can
// still be started or dismissed; the card only says they are expired.
func (c *Coordinator) SupersedeSuggestions(mainThreadID, exceptEventID string) {
	main, err := c.store.GetThread(mainThreadID)
	if err != nil {
		return
	}
	events, err := c.store.ListThreadEvents(mainThreadID)
	if err != nil {
		return
	}
	c.cards.Lock()
	defer c.cards.Unlock()
	for i := range events {
		m := &events[i]
		if m.Kind != db.EvSuggestions || m.ID == exceptEventID {
			continue
		}
		var pl suggestionsPayload
		if json.Unmarshal([]byte(m.Payload), &pl) != nil {
			continue
		}
		changed := false
		for j := range pl.Suggestions {
			s := &pl.Suggestions[j]
			if s.ThreadID != "" || s.Dismissed || s.Superseded {
				continue
			}
			s.Superseded = true
			changed = true
		}
		if !changed {
			continue
		}
		m.Payload = jsonString(pl)
		if c.store.UpdateThreadEvent(m) != nil {
			continue
		}
		c.publishEvent(main.ProjectID, m)
	}
}

// cardEvent loads a proposal card of a main thread.
func (c *Coordinator) cardEvent(mainThreadID, eventID string) (*db.Thread, *db.ThreadEvent, suggestionsPayload, error) {
	main, err := c.store.GetThread(mainThreadID)
	if err != nil {
		return nil, nil, suggestionsPayload{}, err
	}
	m, err := c.store.GetThreadEvent(eventID)
	if err != nil {
		return nil, nil, suggestionsPayload{}, err
	}
	if m.ThreadID != main.ID || m.Kind != db.EvSuggestions {
		return nil, nil, suggestionsPayload{}, db.ErrNotFound
	}
	var pl suggestionsPayload
	if err := json.Unmarshal([]byte(m.Payload), &pl); err != nil {
		return nil, nil, suggestionsPayload{}, err
	}
	return main, m, pl, nil
}

func indexSet(indexes []int) map[int]bool {
	want := map[int]bool{}
	for _, i := range indexes {
		want[i] = true
	}
	return want
}

func cardDeclares(pl suggestionsPayload, id string) bool {
	for _, a := range pl.Actions {
		if a.ID == id {
			return true
		}
	}
	return false
}
