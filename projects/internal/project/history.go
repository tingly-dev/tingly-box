package project

import (
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// Reading history is capped three ways, like grep or ls output: each entry
// is cut at a length, the whole result at a size, and the entry count at a
// number. Whatever does not fit is hidden, the result says so and how to get
// to it (a narrower search, an older cursor, one entry whole by id), so the
// model never mistakes a cut result for the whole.
const (
	readItems    = 20 // default entries per call
	readItemsMax = 40
	// Characters per entry and per result: a preview, or whole (full / by id).
	previewItem  = 600
	previewTotal = 6000
	fullItem     = 4000
	fullTotal    = 12000
	// readScan is how far back a search looks for matches.
	readScan = 1000
)

// readEntry is one line of a history result: an id, who and when, the text.
type readEntry struct{ id, head, body string }

// fit renders entries (oldest first) within the per-entry and total caps. It
// keeps the newest entries, since the cursor continues from the oldest shown,
// and returns the text, the id to continue from, and how many it hid. whole
// says how to read one entry whole.
func fit(entries []readEntry, full bool, whole func(id string) string) (text, oldest string, hidden int) {
	item, total := previewItem, previewTotal
	if full {
		item, total = fullItem, fullTotal
	}
	var lines []string
	used := 0
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		body := strings.ReplaceAll(strings.TrimSpace(e.body), "\n", " ")
		if r := []rune(body); len(r) > item {
			note := fmt.Sprintf(" … [cut, %d more characters", len(r)-item)
			if !full && whole != nil {
				note += "; " + whole(e.id)
			}
			body = string(r[:item]) + note + "]"
		}
		line := fmt.Sprintf("[%s %s] %s", e.id, e.head, body)
		if used+len(line) > total && len(lines) > 0 {
			hidden = i + 1
			break
		}
		used += len(line)
		lines = append(lines, line)
		oldest = e.id
	}
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return strings.Join(lines, "\n"), oldest, hidden
}

// footer says what a capped result left out.
func footer(hidden int, more bool, oldest string, tool string) string {
	var parts []string
	if hidden > 0 {
		parts = append(parts, fmt.Sprintf("%d older entries hidden to keep the result small", hidden))
	}
	if hidden > 0 || more {
		parts = append(parts, fmt.Sprintf("earlier ones exist: narrow with contains, or call %s again with before=%s", tool, oldest))
	}
	if len(parts) == 0 {
		return ""
	}
	return "\n(" + strings.Join(parts, "; ") + ")"
}

// olderNote tells the coordinator its history window is cut, so what it
// cannot see is a thing it can go and read, not a thing that never happened.
const olderNote = "\n\nThe conversation shown is only its latest part; earlier messages exist. When the user refers to something you cannot see, or you need what was decided before, read it with read_conversation instead of guessing or asking again."

// readArgs are the parameters of read_conversation.
type readArgs struct {
	Before, Contains, ID string
	Limit                int
	Full                 bool
}

// readConversation returns earlier events of the project's conversation, the
// ones before the window a turn sees, as quoted history. Before is the
// 8-character id of the event to read back from (default: the oldest event
// of the window), Contains keeps only events with that text, ID reads one
// event whole.
func (c *Coordinator) readConversation(p *db.Project, a readArgs) (string, error) {
	if id := strings.TrimSpace(a.ID); id != "" {
		e, err := c.store.FindThreadEvent(p.MainThreadID, id)
		if err != nil {
			return "", fmt.Errorf("no message %q in this conversation", id)
		}
		text, _, _ := fit([]readEntry{{short(e.ID), historyWho(*e) + " " + stamp(e.CreatedAt), e.Content}}, true, nil)
		return envelope(trustRelay, text, "source", "history", "kind", "read_conversation"), nil
	}
	limit := a.Limit
	if limit <= 0 {
		limit = readItems
	}
	if limit > readItemsMax {
		limit = readItemsMax
	}
	before := strings.TrimSpace(a.Before)
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
	contains := strings.ToLower(strings.TrimSpace(a.Contains))
	if contains != "" {
		scan = readScan
	}
	events, more, err := c.store.PageThreadEvents(p.MainThreadID, db.EventPage{Before: before, Limit: scan})
	if err != nil {
		return "", err
	}
	var kept []readEntry
	for _, e := range events {
		if e.Kind == db.EvUsage || e.Kind == db.EvTask || strings.TrimSpace(e.Content) == "" {
			continue
		}
		if contains != "" && !strings.Contains(strings.ToLower(e.Content), contains) {
			continue
		}
		kept = append(kept, readEntry{short(e.ID), historyWho(e) + " " + stamp(e.CreatedAt), e.Content})
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
	text, oldest, hidden := fit(kept, a.Full, func(id string) string { return "read_conversation id=" + id + " for the whole" })
	return envelope(trustRelay, text+footer(hidden, more, oldest, "read_conversation"), "source", "history", "kind", "read_conversation"), nil
}

// listThreads lists the project's threads, newest activity first, including
// the old and archived ones the thread table leaves out.
func (c *Coordinator) listThreads(p *db.Project, status, contains string, limit int) (string, error) {
	threads, err := c.store.ListThreads(p.ID)
	if err != nil {
		return "", err
	}
	if limit <= 0 {
		limit = readItems
	}
	if limit > readItemsMax {
		limit = readItemsMax
	}
	status, contains = strings.TrimSpace(status), strings.ToLower(strings.TrimSpace(contains))
	var lines []string
	used, matched := 0, 0
	for _, t := range threads {
		if status != "" && t.Status != status {
			continue
		}
		if contains != "" && !strings.Contains(strings.ToLower(t.Title+" "+t.Task+" "+t.LastReport), contains) {
			continue
		}
		matched++
		line := fmt.Sprintf("- %s %q status=%s updated=%s", short(t.ID), t.Title, t.Status, stamp(t.UpdatedAt))
		if t.LastReport != "" {
			line += " last report: " + clip(strings.ReplaceAll(t.LastReport, "\n", " "), 200)
		}
		if len(lines) >= limit || (used+len(line) > previewTotal && len(lines) > 0) {
			continue
		}
		used += len(line)
		lines = append(lines, line)
	}
	if matched == 0 {
		return "no thread matches", nil
	}
	out := strings.Join(lines, "\n")
	if hidden := matched - len(lines); hidden > 0 {
		out += fmt.Sprintf("\n(%d more match and are hidden to keep the result small; narrow with status or contains)", hidden)
	}
	return envelope(trustRelay, out, "source", "history", "kind", "list_threads"), nil
}

// historyWho names who spoke an event, as the coordinator reads it back.
func historyWho(e db.ThreadEvent) string {
	switch {
	case e.Kind == db.EvUser && (e.Source == db.SourceThread || e.Source == db.SourceSystem):
		return "system"
	case e.Kind == db.EvUser:
		return "user"
	case e.Kind == db.EvAssistant:
		return "you"
	}
	return e.Kind
}
