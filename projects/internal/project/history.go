package project

import (
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

const (
	// readLimitDefault and readLimitMax bound one read_conversation call.
	readLimitDefault = 20
	readLimitMax     = 40
	// readScan is how far back a search looks for matches.
	readScan = 1000
)

// olderNote tells the coordinator its history window is cut, so what it
// cannot see is a thing it can go and read, not a thing that never happened.
const olderNote = "\n\nThe conversation shown is only its latest part; earlier messages exist. When the user refers to something you cannot see, or you need what was decided before, read it with read_conversation instead of guessing or asking again."

// readConversation returns older events of the project's conversation, the
// ones before the window a turn sees, as quoted history. before is the
// 8-character id of the event to read back from (default: the oldest event
// of the window); contains keeps only events with that text.
func (c *Coordinator) readConversation(p *db.Project, before, contains string, limit int) (string, error) {
	if limit <= 0 {
		limit = readLimitDefault
	}
	if limit > readLimitMax {
		limit = readLimitMax
	}
	before = strings.TrimSpace(before)
	if before == "" {
		window, err := c.store.ListRecentThreadEvents(p.MainThreadID, historyLimit)
		if err != nil {
			return "", err
		}
		if len(window) == 0 {
			return "the conversation is empty", nil
		}
		before = window[0].ID
	} else {
		e, err := c.store.FindThreadEvent(p.MainThreadID, before)
		if err != nil {
			return "", fmt.Errorf("no message %q in this conversation", before)
		}
		before = e.ID
	}
	scan := limit
	contains = strings.ToLower(strings.TrimSpace(contains))
	if contains != "" {
		scan = readScan
	}
	events, more, err := c.store.PageThreadEvents(p.MainThreadID, db.EventPage{Before: before, Limit: scan})
	if err != nil {
		return "", err
	}
	var kept []db.ThreadEvent
	for _, e := range events {
		if e.Kind == db.EvUsage || e.Kind == db.EvTask || strings.TrimSpace(e.Content) == "" {
			continue
		}
		if contains != "" && !strings.Contains(strings.ToLower(e.Content), contains) {
			continue
		}
		kept = append(kept, e)
	}
	// With a search the page is the scan window; the answer is still the
	// nearest limit matches.
	if len(kept) > limit {
		kept = kept[len(kept)-limit:]
		more = true
	}
	if len(kept) == 0 {
		if more {
			return "no match in the part searched; the conversation goes back further", nil
		}
		if contains != "" {
			return "nothing earlier matches", nil
		}
		return "nothing earlier", nil
	}
	var b strings.Builder
	for _, e := range kept {
		who := e.Kind
		switch {
		case e.Kind == db.EvUser && e.Source == db.SourceThread, e.Kind == db.EvUser && e.Source == db.SourceSystem:
			who = "system"
		case e.Kind == db.EvUser:
			who = "user"
		case e.Kind == db.EvAssistant:
			who = "you"
		}
		fmt.Fprintf(&b, "[%s %s %s] %s\n", short(e.ID), who, stamp(e.CreatedAt), clip(strings.ReplaceAll(strings.TrimSpace(e.Content), "\n", " "), 600))
	}
	if more {
		fmt.Fprintf(&b, "(earlier still exists: call again with before=%s)", short(kept[0].ID))
	}
	return envelope(trustRelay, b.String(), "source", "history", "kind", "read_conversation"), nil
}
