package project

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/remote/agent"
)

// Threads the coordinator proposes (a suggestions card) are offered to the
// project's chats like a decision: a button for each, one for all, or the reply
// "start 1,3" / "start all". One answer settles the card in the chats; whatever
// is left can be started on the web.

const (
	// choiceStartAll is the value of the button that starts every proposal.
	choiceStartAll = "sall"
	// maxSuggestionButtons is how many proposals get a button of their own.
	maxSuggestionButtons = 6
)

// choiceStart is the value of the button that starts proposal i.
func choiceStart(i int) string { return "s" + strconv.Itoa(i) }

// startOfChoice reads a button value made by choiceStart or choiceStartAll: the
// proposals to start, nil for all of them.
func startOfChoice(value string) (indexes []int, ok bool) {
	if value == choiceStartAll {
		return nil, true
	}
	n, err := strconv.Atoi(strings.TrimPrefix(value, "s"))
	return []int{n}, err == nil && strings.HasPrefix(value, "s") && n >= 0
}

// startOfReply reads a reply such as "start 1,3" or "start all" as proposals
// to start (nil for all). ok is false for anything else, and for numbers that
// are not among the n proposals.
func startOfReply(n int, text string) (indexes []int, ok bool) {
	rest, found := strings.CutPrefix(strings.ToLower(strings.TrimSpace(text)), "start")
	if !found || (rest != "" && !strings.ContainsAny(rest[:1], " \t:,.")) {
		return nil, false
	}
	rest = strings.Trim(rest, " \t:,.!")
	if rest == "all" || rest == "everything" {
		return nil, true
	}
	fields := strings.FieldsFunc(rest, func(r rune) bool { return r == ',' || r == ' ' || r == '，' || r == '、' || r == '#' })
	if len(fields) == 0 {
		return nil, false
	}
	seen := map[int]bool{}
	for _, f := range fields {
		k, err := strconv.Atoi(f)
		if err != nil || k < 1 || k > n {
			return nil, false
		}
		if !seen[k-1] {
			seen[k-1] = true
			indexes = append(indexes, k-1)
		}
	}
	return indexes, true
}

// pendingSuggestions returns a suggestions card of a main thread that still
// has proposals not started.
func (c *Coordinator) pendingSuggestions(threadID, eventID string) (db.ThreadEvent, []Suggestion, bool) {
	m, err := c.store.GetThreadEvent(eventID)
	if err != nil || m.ThreadID != threadID || m.Kind != db.EvSuggestions {
		return db.ThreadEvent{}, nil, false
	}
	var pl suggestionsPayload
	if json.Unmarshal([]byte(m.Payload), &pl) != nil {
		return db.ThreadEvent{}, nil, false
	}
	for _, s := range pl.Suggestions {
		if s.ThreadID == "" {
			return *m, pl.Suggestions, true
		}
	}
	return db.ThreadEvent{}, nil, false
}

// suggestions offers the proposed threads of a suggestions card to the chats
// of the project.
func (r *Relay) suggestions(e db.ThreadEvent) {
	var pl suggestionsPayload
	if json.Unmarshal([]byte(e.Payload), &pl) != nil || len(pl.Suggestions) == 0 {
		return
	}
	t, err := r.store.GetThread(e.ThreadID)
	if err != nil {
		return
	}
	var lines []string
	choices := []agent.Choice{{Value: choiceStartAll, Label: "Start all"}}
	for i, s := range pl.Suggestions {
		if s.ThreadID != "" {
			continue
		}
		line := fmt.Sprintf("%d. %s", i+1, s.Title)
		if task := oneLine(s.Task); task != "" && task != s.Title {
			line += ": " + task
		}
		lines = append(lines, line)
		if len(pl.Suggestions) <= maxSuggestionButtons {
			choices = append(choices, agent.Choice{Value: choiceStart(i), Label: fmt.Sprintf("Start %d: %s", i+1, s.Title)})
		}
	}
	if len(lines) == 0 {
		return
	}
	body := "Proposed threads:\n" + strings.Join(lines, "\n")
	r.enqueueJob(relayJob{
		threadIDs:        []string{e.ThreadID},
		requestProjectID: t.ProjectID,
		text:             body + "\nReply \"start 1,3\" or \"start all\" to start them.",
		quote:            "Re: Proposed threads",
		request:          &pendingRequest{threadID: e.ThreadID, requestID: e.ID},
		prompt:           &agent.Prompt{ThreadID: e.ThreadID, RequestID: e.ID, Text: body, Choices: choices},
	})
}

// Suggested restates a suggestions card in the chats it was shown to, with
// the threads started and where, so none offers buttons for it any more.
func (r *Relay) Suggested(e db.ThreadEvent, src Source) {
	if r == nil || r.sink == nil {
		return
	}
	var pl suggestionsPayload
	if json.Unmarshal([]byte(e.Payload), &pl) != nil {
		return
	}
	var started []string
	for _, s := range pl.Suggestions {
		if s.ThreadID == "" {
			continue
		}
		if t, err := r.store.GetThread(s.ThreadID); err == nil {
			started = append(started, threadTag(t.Seq, t.Title))
		}
	}
	if len(started) == 0 {
		return
	}
	r.forget(e.ID)
	where := " on the web"
	if src.Kind == db.SourceIM {
		where = " in a chat"
	}
	r.enqueueJob(relayJob{settle: e.ID, text: join("Started"+where, strings.Join(started, ", "))})
}
