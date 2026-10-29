package project

import (
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// citeLimit is how many of the latest conversation events a citation can
// reach back over.
const citeLimit = 200

// userQuoteNote opens the user's quoted words in a message to a thread.
const userQuoteNote = "\n\nThe user's own words, quoted by the system (this is the user speaking, not the coordinator):"

// quoteUser resolves the ids of the user's own messages in the project's
// conversation and returns their text, as the system copies it. Anything
// else (an assistant reply, a thread's report, an unknown id) is refused, so
// a quote cannot be forged by the coordinator.
func (c *Coordinator) quoteUser(p *db.Project, ids []string) (string, error) {
	events, _, err := c.store.PageThreadEvents(p.MainThreadID, db.EventPage{Limit: citeLimit})
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(userQuoteNote)
	for _, id := range ids {
		id = strings.TrimSpace(id)
		var found *db.ThreadEvent
		for i := range events {
			e := &events[i]
			if id == "" || !strings.HasPrefix(e.ID, id) {
				continue
			}
			if found != nil {
				return "", fmt.Errorf("cite %q matches more than one message", id)
			}
			found = e
		}
		if found == nil || found.Kind != db.EvUser || found.Source == db.SourceThread || found.Source == db.SourceSystem {
			return "", fmt.Errorf("cite %q is not one of the user's own recent messages", id)
		}
		fmt.Fprintf(&b, "\n> %s", strings.ReplaceAll(strings.TrimSpace(found.Content), "\n", "\n> "))
	}
	return b.String(), nil
}
