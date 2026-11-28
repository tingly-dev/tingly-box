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
	// choiceDismissAll is the value of the button that declines every proposal.
	choiceDismissAll = "dall"
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

// dismissOfChoice reads the button value that declines every proposal.
func dismissOfChoice(value string) (indexes []int, ok bool) {
	return nil, value == choiceDismissAll
}

// dismissOfReply reads a reply such as "dismiss 1,3" or "dismiss all" as the
// proposals to decline (nil for all). It is startOfReply under the other verb.
func dismissOfReply(n int, text string) (indexes []int, ok bool) {
	return replyIndexes(n, text, "dismiss")
}

// startOfReply reads a reply such as "start 1,3" or "start all" as proposals
// to start (nil for all). ok is false for anything else, and for numbers that
// are not among the n proposals.
func startOfReply(n int, text string) (indexes []int, ok bool) {
	return replyIndexes(n, text, "start")
}

// replyIndexes reads "<verb> 1,3" or "<verb> all" as proposal indexes (nil
// for all), refusing numbers outside 1..n.
func replyIndexes(n int, text, verb string) (indexes []int, ok bool) {
	rest, found := strings.CutPrefix(strings.ToLower(strings.TrimSpace(text)), verb)
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
		if s.ThreadID == "" && !s.Dismissed && !s.Superseded {
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
		if s.ThreadID != "" || s.Dismissed || s.Superseded {
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
	choices = append(choices, agent.Choice{Value: choiceDismissAll, Label: "Dismiss all"})
	body := "Proposed threads:\n" + strings.Join(lines, "\n")
	r.enqueueJob(relayJob{
		threadIDs:        []string{e.ThreadID},
		requestProjectID: t.ProjectID,
		text:             body + "\nReply \"start 1,3\" or \"start all\" to start them, or \"dismiss 1,3\" to decline.",
		quote:            "Re: Proposed threads",
		request:          &pendingRequest{threadID: e.ThreadID, requestID: e.ID},
		prompt:           &agent.Prompt{ThreadID: e.ThreadID, RequestID: e.ID, Text: body, Choices: choices},
	})
}

// CardSettled restates a proposal card in the chats it was shown to, with
// what happened to each proposal (started, dismissed, expired) and where, so
// none offers buttons for it any more, and stops taking their next message as
// its answer. The web card gets the same change through publishEvent.
func (r *Relay) CardSettled(e db.ThreadEvent, src Source) {
	if r == nil || r.sink == nil {
		return
	}
	var pl suggestionsPayload
	if json.Unmarshal([]byte(e.Payload), &pl) != nil {
		return
	}
	var started, dismissed, expired, left []string
	for _, s := range pl.Suggestions {
		switch {
		case s.ThreadID != "":
			name := s.Title
			if t, err := r.store.GetThread(s.ThreadID); err == nil {
				name = threadTag(t.Seq, t.Title)
			}
			started = append(started, name)
		case s.Dismissed:
			dismissed = append(dismissed, s.Title)
		case s.Superseded:
			expired = append(expired, s.Title)
		default:
			left = append(left, s.Title)
		}
	}
	where := " on the web"
	if src.Kind == db.SourceIM {
		where = " in a chat"
	}
	var lines []string
	if len(started) > 0 {
		lines = append(lines, "Started"+where+": "+strings.Join(started, ", "))
	}
	if len(dismissed) > 0 {
		lines = append(lines, "Dismissed"+where+": "+strings.Join(dismissed, ", "))
	}
	if len(expired) > 0 {
		lines = append(lines, "Expired"+where+": "+strings.Join(expired, ", "))
	}
	if len(left) > 0 {
		// The buttons are gone from the chat copy; the web card still
		// offers what is left.
		lines = append(lines, "Still proposed: "+strings.Join(left, ", ")+" (start it in the web UI).")
	}
	if len(lines) == 0 {
		return
	}
	r.forget(e.ID)
	r.enqueueJob(relayJob{settle: e.ID, text: strings.Join(lines, "\n")})
}
