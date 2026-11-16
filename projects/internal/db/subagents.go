package db

import (
	"encoding/json"
	"time"
)

// Subagent is one Agent call of a thread, for finding it in a long
// transcript: what it was asked, how it ended, how much it did. Its steps
// and its place in the conversation are read when it is opened.
type Subagent struct {
	// Call is the Agent call's request id: its steps' parent.
	Call string `json:"call"`
	// EventID is the call's event, where it sits in the conversation.
	EventID      string `json:"event_id"`
	Description  string `json:"description"`
	SubagentType string `json:"subagent_type,omitempty"`
	// Background says it runs on after its turn.
	Background bool `json:"background"`
	// Status is running, completed, stopped or failed. A foreground run
	// still running after its turn ended was cut off with it; the caller
	// knows whether the turn runs.
	Status    string    `json:"status"`
	Steps     int       `json:"steps"`
	CreatedAt time.Time `json:"created_at"`
}

// ThreadSubagents lists a thread's subagents, oldest first.
func (s *Store) ThreadSubagents(threadID string) ([]Subagent, error) {
	var calls []ThreadEvent
	if err := s.db.Select("id", "request_id", "payload", "created_at").
		Where("thread_id = ? AND parent = '' AND kind = ? AND content IN ? AND request_id != ''", threadID, EvToolUse, []string{"Agent", "Task"}).
		Order("created_at ASC, rowid ASC").Find(&calls).Error; err != nil {
		return nil, err
	}
	out := make([]Subagent, 0, len(calls))
	if len(calls) == 0 {
		return out, nil
	}
	ids := make([]string, len(calls))
	for i, c := range calls {
		ids[i] = c.RequestID
	}
	var counts []struct {
		Parent string
		N      int
	}
	if err := s.db.Model(&ThreadEvent{}).Select("parent, COUNT(*) AS n").
		Where("thread_id = ? AND parent IN ?", threadID, ids).Group("parent").Scan(&counts).Error; err != nil {
		return nil, err
	}
	steps := map[string]int{}
	for _, c := range counts {
		steps[c.Parent] = c.N
	}
	// How each ended: its task events when Claude Code sends them, else
	// whether the call has a result.
	var ends []ThreadEvent
	if err := s.db.Select("request_id", "kind", "payload").
		Where("thread_id = ? AND parent = '' AND kind IN ? AND request_id IN ?", threadID, []string{EvTask, EvToolResult}, ids).
		Order("created_at ASC, rowid ASC").Find(&ends).Error; err != nil {
		return nil, err
	}
	type state struct {
		status     string
		background bool
		kind       string
		result     bool
	}
	states := map[string]*state{}
	for _, e := range ends {
		st := states[e.RequestID]
		if st == nil {
			st = &state{}
			states[e.RequestID] = st
		}
		if e.Kind == EvToolResult {
			st.result = true
			continue
		}
		var ev struct {
			Event        string `json:"event"`
			Status       string `json:"status"`
			Background   bool   `json:"background"`
			SubagentType string `json:"subagent_type"`
		}
		if json.Unmarshal([]byte(e.Payload), &ev) != nil {
			continue
		}
		switch ev.Event {
		case "task_started":
			st.background = ev.Background
			if ev.SubagentType != "" {
				st.kind = ev.SubagentType
			}
			if st.status == "" {
				st.status = "running"
			}
		case "task_updated":
			if ev.Status != "" {
				st.status = finalStatus(ev.Status)
			}
		case "task_notification", "task_completed":
			st.status = finalStatus(ev.Status)
		}
	}
	for _, c := range calls {
		var in struct {
			Description  string `json:"description"`
			SubagentType string `json:"subagent_type"`
		}
		_ = json.Unmarshal([]byte(c.Payload), &in)
		a := Subagent{Call: c.RequestID, EventID: c.ID, Description: in.Description, SubagentType: in.SubagentType, Status: "running", Steps: steps[c.RequestID], CreatedAt: c.CreatedAt}
		if st := states[c.RequestID]; st != nil {
			a.Background = st.background
			if st.kind != "" {
				a.SubagentType = st.kind
			}
			switch {
			case st.status != "":
				a.Status = st.status
			case st.result:
				a.Status = "completed"
			}
		}
		out = append(out, a)
	}
	return out, nil
}

// finalStatus names a task's end the way the transcript does.
func finalStatus(s string) string {
	switch s {
	case "", "completed":
		return "completed"
	case "stopped", "killed":
		return "stopped"
	case "failed", "error":
		return "failed"
	}
	return "running"
}
