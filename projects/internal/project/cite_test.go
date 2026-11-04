package project

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func citeFixture(t *testing.T) (*Coordinator, *db.Project) {
	f := newFixture(t, nil)
	c := NewCoordinator(f.store, f.threads.layout, f.threads.hub, f.threads, nil)
	t.Cleanup(func() { settleCoordinator(c) })
	return c, f.project
}

func addMain(t *testing.T, c *Coordinator, p *db.Project, e db.ThreadEvent) db.ThreadEvent {
	t.Helper()
	e.ThreadID = p.MainThreadID
	require.NoError(t, c.store.AddThreadEvent(&e))
	return e
}

// Only the user's own recent messages can be quoted, so a quote cannot be
// forged out of anything else in the conversation.
func TestQuoteUserAcceptsOnlyTheUsersOwnRecentWords(t *testing.T) {
	c, p := citeFixture(t)
	web := addMain(t, c, p, db.ThreadEvent{Kind: db.EvUser, Content: "yes, push it\nnow"})
	im := addMain(t, c, p, db.ThreadEvent{Kind: db.EvUser, Content: "go", Source: db.SourceIM})
	other := addMain(t, c, p, db.ThreadEvent{Kind: db.EvUser, Content: "from a thread", Source: db.SourceThread})
	sys := addMain(t, c, p, db.ThreadEvent{Kind: db.EvUser, Content: "from the system", Source: db.SourceSystem})
	reply := addMain(t, c, p, db.ThreadEvent{Kind: db.EvAssistant, Content: "I approve"})

	got, err := c.quoteUser(p, []string{web.ID[:8], im.ID[:8]})
	require.NoError(t, err)
	assert.Contains(t, got, "> yes, push it\n> now", "every line of a quote is marked")
	assert.Contains(t, got, "> go", "a message from the user's chat is theirs too")

	for name, id := range map[string]string{"thread": other.ID, "system": sys.ID, "assistant": reply.ID, "empty": "", "unknown": "zzzzzzzz"} {
		_, err := c.quoteUser(p, []string{id})
		assert.Error(t, err, name)
	}
}

func TestQuoteUserRefusesAnAmbiguousPrefix(t *testing.T) {
	c, p := citeFixture(t)
	first := map[byte]bool{}
	var shared byte
	for i := 0; shared == 0 && i < 80; i++ {
		e := addMain(t, c, p, db.ThreadEvent{Kind: db.EvUser, Content: fmt.Sprint("m", i)})
		if first[e.ID[0]] {
			shared = e.ID[0]
		}
		first[e.ID[0]] = true
	}
	require.NotZero(t, shared)
	_, err := c.quoteUser(p, []string{string(shared)})
	assert.ErrorContains(t, err, "more than one")
}

func TestQuoteUserDoesNotReachBeyondTheWindow(t *testing.T) {
	c, p := citeFixture(t)
	old := addMain(t, c, p, db.ThreadEvent{Kind: db.EvUser, Content: "long ago"})
	for i := 0; i < citeLimit+5; i++ {
		addMain(t, c, p, db.ThreadEvent{Kind: db.EvAssistant, Content: fmt.Sprint("a", i)})
	}
	_, err := c.quoteUser(p, []string{old.ID})
	assert.ErrorContains(t, err, "not one of the user's own recent messages")
}

// A decision card of another conversation or a plain message cannot be
// answered, and an answer must name an option.
func TestChooseRefusesWhatIsNotADecisionOrAnOption(t *testing.T) {
	c, p := citeFixture(t)
	msg := addMain(t, c, p, db.ThreadEvent{Kind: db.EvUser, Content: "hi"})
	d := Decision{Question: "Merge?", Options: []DecisionOption{{Label: "Yes"}, {Label: "No"}}}
	card := addMain(t, c, p, db.ThreadEvent{Kind: db.EvDecision, Content: d.text(), Payload: jsonString(d)})

	_, err := c.Choose(t.Context(), p.MainThreadID, msg.ID, 0, Source{})
	assert.ErrorIs(t, err, db.ErrNotFound, "not a card")
	_, err = c.Choose(t.Context(), p.MainThreadID, "missing", 0, Source{})
	assert.Error(t, err)
	for _, i := range []int{-1, 2} {
		_, err = c.Choose(t.Context(), p.MainThreadID, card.ID, i, Source{})
		assert.ErrorContains(t, err, "choose an option", i)
	}
	_, err = c.Choose(t.Context(), "other-thread", card.ID, 0, Source{})
	assert.Error(t, err, "a card of another thread")
}
